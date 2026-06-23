// Package pkgsite provides a client for the pkg.go.dev v1beta API.
package pkgsite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultServer is the default pkgsite API server.
	DefaultServer    = "https://pkg.go.dev"
	DefaultUserAgent = "pkgsite-go"
	apiVersion       = "v1beta"
)

// Client fetches data from the pkg.go.dev v1beta API.
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
	if c.userAgent == "" {
		c.userAgent = DefaultUserAgent
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

var _ error = (*Error)(nil)

// Error is the error format returned by the v1beta API.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	// Fixes is suggestions that tell you how to deal with this error.
	Fixes      []string    `json:"fixes"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Candidate is a module/package candidate returned for ambiguous package paths.
type Candidate struct {
	ModulePath  string `json:"modulePath"`
	PackagePath string `json:"packagePath"`
}

func (e *Error) Error() string {
	status := ""
	if e.Code >= 100 {
		status = fmt.Sprintf(" (HTTP %d)", e.Code)
	}
	if len(e.Candidates) > 0 {
		var builder strings.Builder
		fmt.Fprintf(&builder, "%s%s; specify module path:\n", e.Message, status)
		for _, candidate := range e.Candidates {
			fmt.Fprintf(&builder, "  %s\n", candidate.ModulePath)
		}
		writeFixes(&builder, e.Fixes)
		return builder.String()
	}
	if len(e.Fixes) > 0 {
		var builder strings.Builder
		fmt.Fprintf(&builder, "%s%s", e.Message, status)
		writeFixes(&builder, e.Fixes)
		return builder.String()
	}
	return fmt.Sprintf("%s%s", e.Message, status)
}

func writeFixes(builder *strings.Builder, fixes []string) {
	if len(fixes) == 0 {
		return
	}
	fmt.Fprint(builder, "\nTo fix:\n")
	for _, fix := range fixes {
		fmt.Fprintf(builder, "  - %s\n", fix)
	}
}

var _ error = HTTPError(0)

// HTTPError is the error indicated by HTTP status code.
type HTTPError int

func (h HTTPError) Error() string {
	return fmt.Sprintf("%s (HTTP %d)", http.StatusText(int(h)), h)
}

// Package is the JSON response for /v1beta/package/.
type Package struct {
	Path              string    `json:"path"`
	Name              string    `json:"name"`
	ModulePath        string    `json:"modulePath"`
	Version           string    `json:"version"`
	Synopsis          string    `json:"synopsis"`
	IsRedistributable bool      `json:"isRedistributable"`
	IsStandardLibrary bool      `json:"isStandardLibrary"`
	IsLatest          bool      `json:"isLatest"`
	GOOS              string    `json:"goos"`
	GOARCH            string    `json:"goarch"`
	Docs              string    `json:"docs,omitempty"`
	Imports           []string  `json:"imports,omitempty"`
	Licenses          []License `json:"licenses,omitempty"`
}

// PackageInfo is package metadata returned by package list endpoints.
type PackageInfo struct {
	Path              string `json:"path"`
	Name              string `json:"name"`
	Synopsis          string `json:"synopsis"`
	IsRedistributable bool   `json:"isRedistributable"`
}

// PackagesResponse is the JSON response for /v1beta/packages/.
type PackagesResponse struct {
	ModulePath        string                         `json:"modulePath"`
	Version           string                         `json:"version"`
	IsStandardLibrary bool                           `json:"isStandardLibrary"`
	Packages          PaginatedResponse[PackageInfo] `json:"packages"`
}

// License is license metadata returned by package and module endpoints.
type License struct {
	Types    []string `json:"types"`
	FilePath string   `json:"filePath"`
	Contents string   `json:"contents,omitempty"`
}

var _ queryOptions = (*PackageOptions)(nil)

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
	// Filter is a boolean Go expression used by list endpoints such as symbols
	// and imported-by to filter the items in the response. Only items for which
	// the expression evaluates to true are returned.
	//
	// The available variables are the JSON field names of the items being
	// filtered: for symbols, "name", "kind", "synopsis" and "parent"; for
	// imported-by, the import path is bound to "path". Built-in functions are
	// contains, matches (regexp), hasPrefix and hasSuffix. For example:
	// `contains(name, "Reader")` or `matches(path, "^golang.org/x/")`.
	Filter string
}

func (o *PackageOptions) setToken(token string) {
	o.Token = token
}

// Package fetches package metadata for packagePath.
//
// packageOptions may specify a module path and version to disambiguate or pin
// the package lookup. If Doc is set, the response includes rendered package
// documentation in the requested format: "text", "md", "markdown", or "html".
// Examples may be requested only when Doc is also set. Imports and Licenses
// request the corresponding additional response fields.
//
// Package validates option combinations before sending the request. Invalid
// local options return a regular error. Errors returned by the pkg.go.dev API
// are returned as *Error when the response body contains the API error format,
// or as HTTPError when only the HTTP status is available.
func (c *Client) Package(ctx context.Context, packagePath string, packageOptions *PackageOptions) (*Package, error) {
	q := make(url.Values)
	if packageOptions != nil {
		if packageOptions.Examples && packageOptions.Doc == "" {
			return nil, fmt.Errorf("invalid package options: examples require doc format to be specified")
		}
		if packageOptions.Doc != "" && !validDocFormat(packageOptions.Doc) {
			return nil, fmt.Errorf("invalid package options: bad doc format %q: need one of 'text', 'md', 'markdown' or 'html'", packageOptions.Doc)
		}
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
	u, err := c.endpoint(apiVersion, "package", packagePath)
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

// PaginatedResponse is a generic paginated response.
type PaginatedResponse[T any] struct {
	Items         []T    `json:"items"`
	Total         int    `json:"total"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// PackageSymbols is the JSON response for /v1beta/symbols/.
type PackageSymbols struct {
	ModulePath string                    `json:"modulePath"`
	Version    string                    `json:"version"`
	Symbols    PaginatedResponse[Symbol] `json:"symbols"`
}

// Symbol is a single symbol from /v1beta/symbols/.
type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Synopsis string `json:"synopsis"`
	Parent   string `json:"parent,omitempty"`
}

// Symbols fetches exported symbols for a package.
func (c *Client) Symbols(ctx context.Context, path string, opts *PackageOptions) (*PackageSymbols, error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addString(q, "module", opts.Module)
		addString(q, "goos", opts.GOOS)
		addString(q, "goarch", opts.GOARCH)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
		addString(q, "filter", opts.Filter)
	}
	u, err := c.endpoint(apiVersion, "symbols", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp PackageSymbols
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PackageImportedBy is the response for /v1beta/imported-by/.
type PackageImportedBy struct {
	ModulePath string                    `json:"modulePath"`
	Version    string                    `json:"version"`
	ImportedBy PaginatedResponse[string] `json:"importedBy"`
}

// ImportedBy fetches packages that import path.
func (c *Client) ImportedBy(ctx context.Context, path string, opts *PackageOptions) (*PackageImportedBy, error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addString(q, "module", opts.Module)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
		addString(q, "filter", opts.Filter)
	}
	u, err := c.endpoint(apiVersion, "imported-by", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp PackageImportedBy
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Module is the JSON response for /v1beta/module/.
type Module struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	// CommitTime is the timestamp returned by the module proxy's .info endpoint,
	// representing the time the version was created.
	CommitTime        time.Time `json:"commitTime"`
	IsLatest          bool      `json:"isLatest"`
	IsRedistributable bool      `json:"isRedistributable"`
	IsStandardLibrary bool      `json:"isStandardLibrary"`
	HasGoMod          bool      `json:"hasGoMod"`
	RepoURL           string    `json:"repoUrl"`
	GoModContents     string    `json:"goModContents,omitempty"`
	Readme            *Readme   `json:"readme,omitempty"`
	Licenses          []License `json:"licenses,omitempty"`
}

// Readme is README content returned by /v1beta/module/.
type Readme struct {
	Filepath string `json:"filepath"`
	Contents string `json:"contents"`
}

var _ queryOptions = (*ModuleOptions)(nil)

// ModuleOptions configures module-related requests.
type ModuleOptions struct {
	Version  string
	Module   string
	Readme   bool
	Licenses bool
	Limit    int
	Token    string
	// Filter is a boolean Go expression used by list endpoints (versions,
	// vulns and packages) to filter the items in the response. Only items for
	// which the expression evaluates to true are returned.
	//
	// The available variables are the JSON field names of the items being
	// filtered, such as "path", "name" and "synopsis" for packages, the
	// ModuleVersion fields for versions, or "id", "summary" and "details" for
	// vulns. Built-in functions are contains, matches (regexp), hasPrefix and
	// hasSuffix. For example: `hasPrefix(path, "internal/")`.
	Filter string
}

func (o *ModuleOptions) setToken(token string) {
	o.Token = token
}

// Module fetches module metadata.
func (c *Client) Module(ctx context.Context, path string, opts *ModuleOptions) (*Module, error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addBool(q, "readme", opts.Readme)
		addBool(q, "licenses", opts.Licenses)
	}
	u, err := c.endpoint(apiVersion, "module", path)
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

// ModuleVersion is a single version from /v1beta/versions/.
type ModuleVersion struct {
	ModulePath        string    `json:"modulePath"`
	Version           string    `json:"version"`
	CommitTime        time.Time `json:"commitTime"`
	IsRedistributable bool      `json:"isRedistributable"`
	HasGoMod          bool      `json:"hasGoMod"`
	LatestVersion     string    `json:"latestVersion"`
	Deprecated        bool      `json:"deprecated"`
	DeprecationReason string    `json:"deprecationReason"`
	Retracted         bool      `json:"retracted"`
	RetractionReason  string    `json:"retractionReason"`
}

// Versions fetches module versions.
func (c *Client) Versions(ctx context.Context, path string, opts *ModuleOptions) (*PaginatedResponse[ModuleVersion], error) {
	q := make(url.Values)
	if opts != nil {
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
		addString(q, "filter", opts.Filter)
	}
	u, err := c.endpoint(apiVersion, "versions", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp PaginatedResponse[ModuleVersion]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Vulnerability is a single vulnerability from /v1beta/vulns/.
type Vulnerability struct {
	ID           string `json:"id"`
	Summary      string `json:"summary"`
	Details      string `json:"details"`
	FixedVersion string `json:"fixedVersion"`
}

// Vulnerabilities fetches module vulnerabilities.
func (c *Client) Vulnerabilities(ctx context.Context, path string, opts *ModuleOptions) (*PaginatedResponse[Vulnerability], error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addString(q, "module", opts.Module)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
		addString(q, "filter", opts.Filter)
	}
	u, err := c.endpoint(apiVersion, "vulns", path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp PaginatedResponse[Vulnerability]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Packages fetches packages in a module.
func (c *Client) Packages(ctx context.Context, modulePath string, opts *ModuleOptions) (*PackagesResponse, error) {
	q := make(url.Values)
	if opts != nil {
		addVersion(q, opts.Version)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
		addString(q, "filter", opts.Filter)
	}
	u, err := c.endpoint(apiVersion, "packages", modulePath)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp PackagesResponse
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SearchResult is a single search result from /v1beta/search/.
type SearchResult struct {
	PackagePath string `json:"packagePath"`
	ModulePath  string `json:"modulePath"`
	Version     string `json:"version"`
	Synopsis    string `json:"synopsis"`
}

var _ queryOptions = (*SearchOptions)(nil)

// SearchOptions configures search requests.
type SearchOptions struct {
	Symbol string
	Limit  int
	Token  string
	// Filter is a boolean Go expression used to filter the search results. Only
	// results for which the expression evaluates to true are returned.
	//
	// The available variables are the JSON field names of SearchResult:
	// "packagePath", "modulePath", "version" and "synopsis". Built-in functions
	// are contains, matches (regexp), hasPrefix and hasSuffix. For example:
	// `contains(synopsis, "logging")`.
	Filter string
}

func (o *SearchOptions) setToken(token string) {
	o.Token = token
}

// Search searches packages.
func (c *Client) Search(ctx context.Context, query string, opts *SearchOptions) (*PaginatedResponse[SearchResult], error) {
	q := make(url.Values)
	q.Set("q", query)
	if opts != nil {
		addString(q, "symbol", opts.Symbol)
		addLimit(q, opts.Limit)
		addString(q, "token", opts.Token)
		addString(q, "filter", opts.Filter)
	}
	u, err := c.endpoint(apiVersion, "search")
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()

	var resp PaginatedResponse[SearchResult]
	if err := c.get(ctx, u.String(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// FetchModule requests that pkg.go.dev fetch and index path, triggering the same
// action as the "Request" button shown for an unknown path on the site.
// path may optionally end in "@version" (e.g. "example.com/mod@v1.0.0") to
// request a specific version; otherwise the latest version is fetched.
//
// Unlike the rest of Client's methods, FetchModule does not use the v1beta API: it
// posts to the /fetch/ endpoint, which is not version-prefixed. The call
// blocks until pkg.go.dev confirms path has been indexed, or returns an
// error if the fetch failed or the request timed out.
func (c *Client) FetchModule(ctx context.Context, path string) error {
	u, err := c.endpoint("fetch", path)
	if err != nil {
		return err
	}
	return c.post(ctx, u.String())
}

// post now only for FetchModule
func (c *Client) post(ctx context.Context, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, nil)
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
			return HTTPError(resp.StatusCode)
		}
		// The /fetch/ endpoint reports failures as a plain text (sometimes
		// HTML) body rather than the JSON format used by the v1beta API.
		if message := strings.TrimSpace(string(body)); message != "" {
			return &Error{Code: resp.StatusCode, Message: message}
		}
		return HTTPError(resp.StatusCode)
	}
	return nil
}

func (c *Client) get(ctx context.Context, rawURL string, dst any) error {
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
			return HTTPError(resp.StatusCode)
		}
		var apiErr Error
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
			if apiErr.Code == 0 {
				apiErr.Code = resp.StatusCode
			}
			return &apiErr
		}
		return HTTPError(resp.StatusCode)
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
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
}

func validDocFormat(format string) bool {
	switch format {
	case "text", "md", "markdown", "html":
		return true
	default:
		return false
	}
}

type queryOptions interface {
	setToken(token string)
}

func paginateSeq[T any, O queryOptions](
	ctx context.Context,
	options O,
	fetch func(context.Context, O) (items []T, nextToken string, err error),
) iter.Seq2[[]T, error] {
	return func(yield func([]T, error) bool) {
		for {
			items, nextToken, err := fetch(ctx, options)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(items, nil) {
				return
			}
			if nextToken == "" {
				return
			}
			options.setToken(nextToken)
		}
	}
}

// SymbolsIter returns an iterator for paginating through symbols.
// The iterator yields pages of symbols. If Next returns an error, the next call
// to Next will retry the same page.
func (c *Client) SymbolsIter(ctx context.Context, path string, options *PackageOptions) iter.Seq2[[]Symbol, error] {
	optionsCopy := PackageOptions{}
	if options != nil {
		optionsCopy = *options
	}
	return paginateSeq(ctx, &optionsCopy,
		func(ctx context.Context, currentOptions *PackageOptions) ([]Symbol, string, error) {
			page, err := c.Symbols(ctx, path, currentOptions)
			if err != nil {
				return nil, "", err
			}
			return page.Symbols.Items, page.Symbols.NextPageToken, nil
		},
	)
}

// ImportedByIter returns an iterator for paginating through imported-by packages.
// The iterator yields pages of package paths. If Next returns an error, the next call
// to Next will retry the same page.
func (c *Client) ImportedByIter(ctx context.Context, path string, options *PackageOptions) iter.Seq2[[]string, error] {
	optionsCopy := PackageOptions{}
	if options != nil {
		optionsCopy = *options
	}
	return paginateSeq(ctx, &optionsCopy,
		func(ctx context.Context, currentOptions *PackageOptions) ([]string, string, error) {
			result, err := c.ImportedBy(ctx, path, currentOptions)
			if err != nil {
				return nil, "", err
			}
			return result.ImportedBy.Items, result.ImportedBy.NextPageToken, nil
		},
	)
}

// VersionsIter returns an iterator for paginating through module versions.
// The iterator yields pages of versions. If Next returns an error, the next call
// to Next will retry the same page.
func (c *Client) VersionsIter(ctx context.Context, path string, options *ModuleOptions) iter.Seq2[[]ModuleVersion, error] {
	optionsCopy := ModuleOptions{}
	if options != nil {
		optionsCopy = *options
	}
	return paginateSeq(ctx, &optionsCopy,
		func(ctx context.Context, currentOptions *ModuleOptions) ([]ModuleVersion, string, error) {
			page, err := c.Versions(ctx, path, currentOptions)
			if err != nil {
				return nil, "", err
			}
			return page.Items, page.NextPageToken, nil
		},
	)
}

// VulnerabilitiesIter returns an iterator for paginating through vulnerabilities.
// The iterator yields pages of vulnerabilities. If Next returns an error, the next call
// to Next will retry the same page.
func (c *Client) VulnerabilitiesIter(ctx context.Context, path string, options *ModuleOptions) iter.Seq2[[]Vulnerability, error] {
	optionsCopy := ModuleOptions{}
	if options != nil {
		optionsCopy = *options
	}
	return paginateSeq(ctx, &optionsCopy,
		func(ctx context.Context, currentOptions *ModuleOptions) ([]Vulnerability, string, error) {
			page, err := c.Vulnerabilities(ctx, path, currentOptions)
			if err != nil {
				return nil, "", err
			}
			return page.Items, page.NextPageToken, nil
		},
	)
}

// PackagesIter returns an iterator for paginating through packages in a module.
// The iterator yields pages of packages. If Next returns an error, the next call
// to Next will retry the same page.
func (c *Client) PackagesIter(ctx context.Context, modulePath string, options *ModuleOptions) iter.Seq2[[]PackageInfo, error] {
	optionsCopy := ModuleOptions{}
	if options != nil {
		optionsCopy = *options
	}
	return paginateSeq(ctx, &optionsCopy,
		func(ctx context.Context, currentOptions *ModuleOptions) ([]PackageInfo, string, error) {
			page, err := c.Packages(ctx, modulePath, currentOptions)
			if err != nil {
				return nil, "", err
			}
			return page.Packages.Items, page.Packages.NextPageToken, nil
		},
	)
}

// SearchIter returns an iterator for paginating through search results.
// The iterator yields pages of search results. If Next returns an error, the next call
// to Next will retry the same page.
func (c *Client) SearchIter(ctx context.Context, query string, options *SearchOptions) iter.Seq2[[]SearchResult, error] {
	optionsCopy := SearchOptions{}
	if options != nil {
		optionsCopy = *options
	}
	return paginateSeq(ctx, &optionsCopy,
		func(ctx context.Context, currentOptions *SearchOptions) ([]SearchResult, string, error) {
			page, err := c.Search(ctx, query, currentOptions)
			if err != nil {
				return nil, "", err
			}
			return page.Items, page.NextPageToken, nil
		},
	)
}
