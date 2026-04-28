// Package pkgsite provides a client for the pkg.go.dev v1 API.
package pkgsite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultServer is the default pkgsite API server.
	DefaultServer = "https://pkg.go.dev"
)

// Client fetches data from the pkg.go.dev v1 API.
type Client struct {
	server     string
	httpClient *http.Client
	userAgent  string
}

// NewClient returns a client configured with opts.
func NewClient(options ...Option) *Client {
	c := &Client{
		server:     DefaultServer,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, option := range options {
		option(c)
	}
	if c.server == "" {
		c.server = DefaultServer
	}
	if c.httpClient == nil {
		c.httpClient = http.DefaultClient
	}
	return c
}

// Option configures a Client.
type Option func(*Client)

// WithServer configures the pkgsite API server URL.
func WithServer(server string) Option {
	return func(c *Client) {
		c.server = server
	}
}

// WithHTTPClient configures the HTTP client used for requests.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithUserAgent configures the HTTP User-Agent header.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		c.userAgent = userAgent
	}
}

// APIError is the error format returned by the v1 API.
type APIError struct {
	Code       int         `json:"code"`
	Message    string      `json:"message"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Candidate is a module/package candidate returned for ambiguous package paths.
type Candidate struct {
	ModulePath  string `json:"modulePath"`
	PackagePath string `json:"packagePath"`
}

func (e *APIError) Error() string {
	if len(e.Candidates) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%s; specify module:\n", e.Message)
		for _, c := range e.Candidates {
			fmt.Fprintf(&b, "  %s\n", c.ModulePath)
		}
		return b.String()
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Code)
}

// Package is the JSON response for /v1/package/.
type Package struct {
	Path              string    `json:"path"`
	ModulePath        string    `json:"modulePath"`
	ModuleVersion     string    `json:"moduleVersion"`
	Synopsis          string    `json:"synopsis"`
	IsStandardLibrary bool      `json:"isStandardLibrary"`
	IsLatest          bool      `json:"isLatest"`
	GOOS              string    `json:"goos"`
	GOARCH            string    `json:"goarch"`
	Docs              string    `json:"docs,omitempty"`
	Imports           []string  `json:"imports,omitempty"`
	Licenses          []License `json:"licenses,omitempty"`
}

// License is license metadata returned by package and module endpoints.
type License struct {
	Types    []string `json:"types"`
	FilePath string   `json:"filePath"`
	Contents string   `json:"contents,omitempty"`
}

// PackageOptions configures package-related requests.
type PackageOptions struct {
	Version  string
	Module   string
	Doc      string
	Examples bool
	Imports  bool
	Licenses bool
	GOOS     string
	GOARCH   string
	Limit    int
	Token    string
}

// Package fetches package metadata.
func (c *Client) Package(ctx context.Context, packagePath string, packageOptions *PackageOptions) (*Package, error) {
	q := make(url.Values)
	if packageOptions != nil {
		addVersion(q, packageOptions.Version)
		if packageOptions.Module != "" {
			q.Set("module", packageOptions.Module)
		}
		if packageOptions.Doc != "" {
			q.Set("doc", packageOptions.Doc)
		}
		addBool(q, "examples", packageOptions.Examples)
		addBool(q, "imports", packageOptions.Imports)
		addBool(q, "licenses", packageOptions.Licenses)
		addString(q, "goos", packageOptions.GOOS)
		addString(q, "goarch", packageOptions.GOARCH)
	}
	u, err := c.endpoint("v1", "package", packagePath)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp Package
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Page is a generic paginated response.
type Page[T any] struct {
	Items         []T    `json:"items"`
	Total         int    `json:"total"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// Symbol is a single symbol from /v1/symbols/.
type Symbol struct {
	ModulePath string `json:"modulePath"`
	Version    string `json:"version"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Synopsis   string `json:"synopsis"`
	Parent     string `json:"parent,omitempty"`
}

// Symbols fetches exported symbols for a package.
func (c *Client) Symbols(ctx context.Context, path string, opts *PackageOptions) (*Page[Symbol], error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addString(q, "module", opts.Module)
		addString(q, "goos", opts.GOOS)
		addString(q, "goarch", opts.GOARCH)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
	} else {
		addLimit(q, 0)
	}
	u, err := c.endpoint("v1", "symbols", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp Page[Symbol]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ImportedBy is the response for /v1/imported-by/.
type ImportedBy struct {
	ModulePath string       `json:"modulePath"`
	Version    string       `json:"version"`
	ImportedBy Page[string] `json:"importedBy"`
}

// ImportedBy fetches packages that import path.
func (c *Client) ImportedBy(ctx context.Context, path string, opts *PackageOptions) (*ImportedBy, error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addString(q, "module", opts.Module)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
	} else {
		addLimit(q, 0)
	}
	u, err := c.endpoint("v1", "imported-by", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp ImportedBy
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Module is the JSON response for /v1/module/.
type Module struct {
	Path              string    `json:"path"`
	Version           string    `json:"version"`
	IsLatest          bool      `json:"isLatest"`
	IsRedistributable bool      `json:"isRedistributable"`
	IsStandardLibrary bool      `json:"isStandardLibrary"`
	HasGoMod          bool      `json:"hasGoMod"`
	RepoURL           string    `json:"repoUrl"`
	Readme            *Readme   `json:"readme,omitempty"`
	Licenses          []License `json:"licenses,omitempty"`
}

// Readme is README content returned by /v1/module/.
type Readme struct {
	Filepath string `json:"filepath"`
	Contents string `json:"contents"`
}

// ModuleOptions configures module-related requests.
type ModuleOptions struct {
	Version  string
	Readme   bool
	Licenses bool
	Limit    int
	Token    string
}

// Module fetches module metadata.
func (c *Client) Module(ctx context.Context, path string, opts *ModuleOptions) (*Module, error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addBool(q, "readme", opts.Readme)
		addBool(q, "licenses", opts.Licenses)
	}
	u, err := c.endpoint("v1", "module", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp Module
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Version is a single version from /v1/versions/.
type Version struct {
	Version string `json:"version"`
}

// Versions fetches module versions.
func (c *Client) Versions(ctx context.Context, path string, opts *ModuleOptions) (*Page[Version], error) {
	q := make(url.Values)
	if opts != nil {
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
	} else {
		addLimit(q, 0)
	}
	u, err := c.endpoint("v1", "versions", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp Page[Version]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Vulnerability is a single vulnerability from /v1/vulns/.
type Vulnerability struct {
	ID           string `json:"id"`
	Summary      string `json:"summary"`
	Details      string `json:"details"`
	FixedVersion string `json:"fixedVersion"`
}

// Vulns fetches module vulnerabilities.
func (c *Client) Vulns(ctx context.Context, path string, opts *ModuleOptions) (*Page[Vulnerability], error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
	} else {
		addLimit(q, 0)
	}
	u, err := c.endpoint("v1", "vulns", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp Page[Vulnerability]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ModulePackage is a single package from /v1/packages/.
type ModulePackage struct {
	Path     string `json:"path"`
	Synopsis string `json:"synopsis"`
}

// Packages fetches packages in a module.
func (c *Client) Packages(ctx context.Context, modulePath string, opts *ModuleOptions) (*Page[ModulePackage], error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
	} else {
		addLimit(q, 0)
	}
	u, err := c.endpoint("v1", "packages", modulePath)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp Page[ModulePackage]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SearchResult is a single search result from /v1/search/.
type SearchResult struct {
	PackagePath string `json:"packagePath"`
	ModulePath  string `json:"modulePath"`
	Version     string `json:"version"`
	Synopsis    string `json:"synopsis"`
}

// SearchOptions configures search requests.
type SearchOptions struct {
	Symbol string
	Limit  int
	Token  string
}

// Search searches packages.
func (c *Client) Search(ctx context.Context, query string, opts *SearchOptions) (*Page[SearchResult], error) {
	q := make(url.Values)
	q.Set("q", query)
	if opts != nil {
		addString(q, "symbol", opts.Symbol)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
	} else {
		addLimit(q, 0)
	}
	u, err := c.endpoint("v1", "search")
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp Page[SearchResult]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) get(ctx context.Context, rawURL string, dst any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		if err != nil {
			return fmt.Errorf("reading error response: %w", err)
		}
		var apiErr APIError
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
			if apiErr.Code == 0 {
				apiErr.Code = resp.StatusCode
			}
			return &apiErr
		}
		return &APIError{
			Code:    resp.StatusCode,
			Message: http.StatusText(resp.StatusCode),
		}
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (c *Client) endpoint(elem ...string) (*url.URL, error) {
	server := c.server
	if server == "" {
		server = DefaultServer
	}
	u, err := url.Parse(server)
	if err != nil {
		return nil, err
	}
	return u.JoinPath(elem...), nil
}

func addVersion(q url.Values, version string) {
	addString(q, "version", version)
}

func addString(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

func addBool(q url.Values, key string, value bool) {
	if value {
		q.Set(key, "true")
	}
}

func addLimit(q url.Values, limit int) {
	if limit <= 0 {
		limit = 100
	}
	q.Set("limit", strconv.Itoa(limit))
}
