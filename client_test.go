package pkgsite

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/package/encoding/json" {
			t.Errorf("path = %q, want /v1beta/package/encoding/json", r.URL.Path)
		}
		if got := r.URL.Query().Get("version"); got != "go1.26.0" {
			t.Errorf("version = %q, want go1.26.0", got)
		}
		json.NewEncoder(w).Encode(Package{
			Path:              "encoding/json",
			Name:              "json",
			ModulePath:        "std",
			Version:           "go1.26.0",
			Synopsis:          "Package json implements encoding and decoding of JSON.",
			IsStandardLibrary: true,
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Package(context.Background(), "encoding/json", &PackageOptions{Version: "go1.26.0"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Path != "encoding/json" {
		t.Errorf("Path = %q, want encoding/json", resp.Path)
	}
	if !resp.IsStandardLibrary {
		t.Error("IsStandardLibrary = false, want true")
	}
}

func TestPackageOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		checkQuery(t, q.Get("doc"), "md", "doc")
		checkQuery(t, q.Get("examples"), "true", "examples")
		checkQuery(t, q.Get("imports"), "true", "imports")
		checkQuery(t, q.Get("licenses"), "true", "licenses")
		checkQuery(t, q.Get("module"), "github.com/foo/bar", "module")
		checkQuery(t, q.Get("goos"), "linux", "goos")
		checkQuery(t, q.Get("goarch"), "amd64", "goarch")
		json.NewEncoder(w).Encode(Package{
			Path:    "github.com/foo/bar/pkg",
			Docs:    "# package pkg",
			Imports: []string{"fmt", "strings"},
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Package(context.Background(), "github.com/foo/bar/pkg", &PackageOptions{
		Doc:      "md",
		Examples: true,
		Imports:  true,
		Licenses: true,
		Module:   "github.com/foo/bar",
		GOOS:     "linux",
		GOARCH:   "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Docs != "# package pkg" {
		t.Errorf("Docs = %q, want # package pkg", resp.Docs)
	}
	if len(resp.Imports) != 2 {
		t.Errorf("len(Imports) = %d, want 2", len(resp.Imports))
	}
}

func TestPackageExamplesRequiresDoc(t *testing.T) {
	c := NewClient()
	_, err := c.Package(context.Background(), "encoding/json", &PackageOptions{Examples: true})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want not *Error", err)
	}
	if got, want := err.Error(), "invalid package options: examples require doc format to be specified"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestPackageInvalidDocFormat(t *testing.T) {
	c := NewClient()
	_, err := c.Package(context.Background(), "encoding/json", &PackageOptions{Doc: "json"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want not *Error", err)
	}
	if got, want := err.Error(), "invalid package options: bad doc format \"json\": need one of 'text', 'md', 'markdown' or 'html'"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestModule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/module/golang.org/x/text" {
			t.Errorf("path = %q, want /v1beta/module/golang.org/x/text", r.URL.Path)
		}
		checkQuery(t, r.URL.Query().Get("version"), "v0.14.0", "version")
		checkQuery(t, r.URL.Query().Get("readme"), "true", "readme")
		checkQuery(t, r.URL.Query().Get("licenses"), "true", "licenses")
		json.NewEncoder(w).Encode(Module{
			Path:    "golang.org/x/text",
			Version: "v0.14.0",
			RepoURL: "https://github.com/golang/text",
			Readme:  &Readme{Filepath: "README.md", Contents: "text"},
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Module(context.Background(), "golang.org/x/text", &ModuleOptions{
		Version:  "v0.14.0",
		Readme:   true,
		Licenses: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Version != "v0.14.0" {
		t.Errorf("Version = %q, want v0.14.0", resp.Version)
	}
}

func TestVersions(t *testing.T) {
	srv := pageServer(t, "/v1beta/versions/golang.org/x/text", "2", PaginatedResponse[ModuleVersion]{
		Items: []ModuleVersion{{Version: "v0.14.0"}, {Version: "v0.13.0"}},
		Total: 2,
	})
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Versions(context.Background(), "golang.org/x/text", &ModuleOptions{Limit: 2, Token: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("len(Items) = %d, want 2", len(resp.Items))
	}
}

func TestVulns(t *testing.T) {
	srv := pageServer(t, "/v1beta/vulns/golang.org/x/text", "", PaginatedResponse[Vulnerability]{
		Items: []Vulnerability{{ID: "GO-2023-0001", Details: "A vulnerability."}},
		Total: 1,
	})
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Vulnerabilities(context.Background(), "golang.org/x/text", &ModuleOptions{Version: "v0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("len(Items) = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].ID != "GO-2023-0001" {
		t.Errorf("ID = %q, want GO-2023-0001", resp.Items[0].ID)
	}
}

func TestPackages(t *testing.T) {
	srv := pageServer(t, "/v1beta/packages/golang.org/x/text", "", PackagesResponse{
		ModulePath:        "golang.org/x/text",
		Version:           "v0.14.0",
		IsStandardLibrary: false,
		Packages: PaginatedResponse[PackageInfo]{
			Items: []PackageInfo{
				{
					Path:              "golang.org/x/text/language",
					Name:              "language",
					Synopsis:          "Package language implements BCP 47 language tags.",
					IsRedistributable: true,
				},
			},
			Total: 1,
		},
	})
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Packages(context.Background(), "golang.org/x/text", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ModulePath != "golang.org/x/text" {
		t.Errorf("ModulePath = %q, want golang.org/x/text", resp.ModulePath)
	}
	if resp.Packages.Items[0].Path != "golang.org/x/text/language" {
		t.Errorf("Path = %q, want golang.org/x/text/language", resp.Packages.Items[0].Path)
	}
	if !resp.Packages.Items[0].IsRedistributable {
		t.Error("IsRedistributable = false, want true")
	}
}

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkQuery(t, r.URL.Path, "/v1beta/search", "path")
		checkQuery(t, r.URL.Query().Get("q"), "json parser", "q")
		checkQuery(t, r.URL.Query().Get("symbol"), "Marshal", "symbol")
		checkQuery(t, r.URL.Query().Get("filter"), "^encoding/", "filter")
		json.NewEncoder(w).Encode(PaginatedResponse[SearchResult]{
			Items: []SearchResult{{
				PackagePath: "encoding/json",
				ModulePath:  "std",
				Version:     "go1.26.0",
				Synopsis:    "Package json implements encoding and decoding of JSON.",
			}},
			Total: 1,
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Search(context.Background(), "json parser", &SearchOptions{Symbol: "Marshal", Filter: "^encoding/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("len(Items) = %d, want 1", len(resp.Items))
	}
}

func TestSymbols(t *testing.T) {
	srv := pageServer(t, "/v1beta/symbols/encoding/json", "", PackageSymbols{
		ModulePath: "std",
		Version:    "go1.26.0",
		Symbols: PaginatedResponse[Symbol]{
			Items: []Symbol{{
				Name:     "Marshal",
				Kind:     "func",
				Synopsis: "func Marshal(v any) ([]byte, error)",
			}},
			Total: 1,
		},
	})
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Symbols(context.Background(), "encoding/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ModulePath != "std" {
		t.Errorf("ModulePath = %q, want std", resp.ModulePath)
	}
	if resp.Symbols.Items[0].Name != "Marshal" {
		t.Errorf("Name = %q, want Marshal", resp.Symbols.Items[0].Name)
	}
}

func TestSymbolsIter(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			json.NewEncoder(w).Encode(PackageSymbols{
				ModulePath: "std",
				Version:    "go1.26.0",
				Symbols: PaginatedResponse[Symbol]{
					Items: []Symbol{
						{Name: "Marshal", Kind: "func"},
						{Name: "Unmarshal", Kind: "func"},
					},
					NextPageToken: "page2",
				},
			})
		} else if count == 2 {
			json.NewEncoder(w).Encode(PackageSymbols{
				ModulePath: "std",
				Version:    "go1.26.0",
				Symbols: PaginatedResponse[Symbol]{
					Items: []Symbol{
						{Name: "MarshalIndent", Kind: "func"},
					},
				},
			})
		}
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	var allSymbols []Symbol
	for symbols, err := range c.SymbolsIter(context.Background(), "encoding/json", nil) {
		if err != nil {
			t.Fatal(err)
		}
		allSymbols = append(allSymbols, symbols...)
	}

	if len(allSymbols) != 3 {
		t.Errorf("len(allSymbols) = %d, want 3", len(allSymbols))
	}
	if allSymbols[0].Name != "Marshal" {
		t.Errorf("first symbol = %q, want Marshal", allSymbols[0].Name)
	}
	if allSymbols[2].Name != "MarshalIndent" {
		t.Errorf("last symbol = %q, want MarshalIndent", allSymbols[2].Name)
	}
}

func TestSymbolsIterWithRetry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(PackageSymbols{
			Symbols: PaginatedResponse[Symbol]{
				Items: []Symbol{{Name: "Marshal", Kind: "func"}},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	iter := c.SymbolsIter(context.Background(), "encoding/json", nil)

	var symbols []Symbol
	var firstErr error
	for syms, err := range iter {
		if err != nil {
			firstErr = err
			// 继续迭代来重试
			continue
		}
		symbols = append(symbols, syms...)
	}

	if firstErr == nil {
		t.Fatal("expected first error")
	}
	if len(symbols) != 1 {
		t.Errorf("len(symbols) = %d, want 1", len(symbols))
	}
}

func TestVersionsIter(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			json.NewEncoder(w).Encode(PaginatedResponse[ModuleVersion]{
				Items:         []ModuleVersion{{Version: "v1.0.0"}, {Version: "v0.9.0"}},
				NextPageToken: "page2",
			})
		} else {
			json.NewEncoder(w).Encode(PaginatedResponse[ModuleVersion]{
				Items: []ModuleVersion{{Version: "v0.8.0"}},
			})
		}
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	var allVersions []ModuleVersion
	for versions, err := range c.VersionsIter(context.Background(), "golang.org/x/text", nil) {
		if err != nil {
			t.Fatal(err)
		}
		allVersions = append(allVersions, versions...)
	}

	if len(allVersions) != 3 {
		t.Errorf("len(allVersions) = %d, want 3", len(allVersions))
	}
	if allVersions[0].Version != "v1.0.0" {
		t.Errorf("first version = %q, want v1.0.0", allVersions[0].Version)
	}
}

func TestPackagesIter(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			json.NewEncoder(w).Encode(PackagesResponse{
				ModulePath: "golang.org/x/text",
				Version:    "v0.14.0",
				Packages: PaginatedResponse[PackageInfo]{
					Items: []PackageInfo{
						{Path: "golang.org/x/text/language"},
						{Path: "golang.org/x/text/encoding"},
					},
					NextPageToken: "page2",
				},
			})
		} else {
			json.NewEncoder(w).Encode(PackagesResponse{
				ModulePath: "golang.org/x/text",
				Version:    "v0.14.0",
				Packages: PaginatedResponse[PackageInfo]{
					Items: []PackageInfo{
						{Path: "golang.org/x/text/unicode"},
					},
				},
			})
		}
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	var allPackages []PackageInfo
	for packages, err := range c.PackagesIter(context.Background(), "golang.org/x/text", nil) {
		if err != nil {
			t.Fatal(err)
		}
		allPackages = append(allPackages, packages...)
	}

	if len(allPackages) != 3 {
		t.Errorf("len(allPackages) = %d, want 3", len(allPackages))
	}
}

func TestSearchIter(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			json.NewEncoder(w).Encode(PaginatedResponse[SearchResult]{
				Items: []SearchResult{
					{PackagePath: "encoding/json", ModulePath: "std"},
				},
				NextPageToken: "page2",
			})
		} else {
			json.NewEncoder(w).Encode(PaginatedResponse[SearchResult]{
				Items: []SearchResult{
					{PackagePath: "encoding/xml", ModulePath: "std"},
				},
			})
		}
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	var allResults []SearchResult
	for results, err := range c.SearchIter(context.Background(), "json", nil) {
		if err != nil {
			t.Fatal(err)
		}
		allResults = append(allResults, results...)
	}

	if len(allResults) != 2 {
		t.Errorf("len(allResults) = %d, want 2", len(allResults))
	}
}

func TestImportedByIter(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			json.NewEncoder(w).Encode(PackageImportedBy{
				ModulePath: "std",
				Version:    "go1.26.0",
				ImportedBy: PaginatedResponse[string]{
					Items:         []string{"github.com/foo/bar", "github.com/baz/qux"},
					NextPageToken: "page2",
				},
			})
		} else {
			json.NewEncoder(w).Encode(PackageImportedBy{
				ModulePath: "std",
				Version:    "go1.26.0",
				ImportedBy: PaginatedResponse[string]{
					Items: []string{"github.com/other/pkg"},
				},
			})
		}
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	var allPkgs []string
	for pkgs, err := range c.ImportedByIter(context.Background(), "encoding/json", nil) {
		if err != nil {
			t.Fatal(err)
		}
		allPkgs = append(allPkgs, pkgs...)
	}

	if len(allPkgs) != 3 {
		t.Errorf("len(allPkgs) = %d, want 3", len(allPkgs))
	}
}

func TestVulnsIter(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			json.NewEncoder(w).Encode(PaginatedResponse[Vulnerability]{
				Items: []Vulnerability{
					{ID: "GO-2023-0001", Summary: "CVE-2023-0001"},
				},
				NextPageToken: "page2",
			})
		} else {
			json.NewEncoder(w).Encode(PaginatedResponse[Vulnerability]{
				Items: []Vulnerability{
					{ID: "GO-2023-0002", Summary: "CVE-2023-0002"},
				},
			})
		}
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	var allVulns []Vulnerability
	for vulns, err := range c.VulnerabilitiesIter(context.Background(), "golang.org/x/text", nil) {
		if err != nil {
			t.Fatal(err)
		}
		allVulns = append(allVulns, vulns...)
	}

	if len(allVulns) != 2 {
		t.Errorf("len(allVulns) = %d, want 2", len(allVulns))
	}
}

func TestImportedBy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkQuery(t, r.URL.Query().Get("filter"), "^github.com/", "filter")
		json.NewEncoder(w).Encode(PackageImportedBy{
			ModulePath: "std",
			Version:    "go1.26.0",
			ImportedBy: PaginatedResponse[string]{
				Items: []string{"github.com/foo/bar", "github.com/baz/qux"},
				Total: 2,
			},
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.ImportedBy(context.Background(), "encoding/json", &PackageOptions{Filter: "^github.com/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ImportedBy.Items) != 2 {
		t.Errorf("len(ImportedBy.Items) = %d, want 2", len(resp.ImportedBy.Items))
	}
}

func TestAmbiguousPackagePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Error{
			Code:    http.StatusBadRequest,
			Message: "ambiguous package path",
			Candidates: []Candidate{
				{ModulePath: "github.com/foo/bar", PackagePath: "github.com/foo/bar/pkg"},
				{ModulePath: "github.com/foo/bar/pkg", PackagePath: "github.com/foo/bar/pkg"},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	_, err := c.Package(context.Background(), "github.com/foo/bar/pkg", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "github.com/foo/bar") {
		t.Errorf("error missing candidate, got:\n%s", msg)
	}
	if !strings.Contains(msg, "github.com/foo/bar/pkg") {
		t.Errorf("error missing candidate, got:\n%s", msg)
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(Error{
			Code:    http.StatusNotFound,
			Message: "not found",
			Fixes:   []string{"check the module or package path"},
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	_, err := c.Package(context.Background(), "nonexistent/pkg", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Code != http.StatusNotFound {
		t.Errorf("Code = %d, want 404", apiErr.Code)
	}
	if len(apiErr.Fixes) != 1 {
		t.Errorf("len(Fixes) = %d, want 1", len(apiErr.Fixes))
	}
}

func TestAPIErrorWithoutJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	_, err := c.Package(context.Background(), "encoding/json", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want HTTPError", err)
	}
	if httpErr != HTTPError(http.StatusTooManyRequests) {
		t.Errorf("HTTPError = %d, want %d", httpErr, http.StatusTooManyRequests)
	}
	if got, want := err.Error(), "Too Many Requests (HTTP 429)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestHTTPErrorWithInvalidJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("server panic"))
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	_, err := c.Package(context.Background(), "encoding/json", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want HTTPError", err)
	}
	if httpErr != HTTPError(http.StatusBadGateway) {
		t.Errorf("HTTPError = %d, want %d", httpErr, http.StatusBadGateway)
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/fetch/path/to/module" {
			t.Errorf("path = %q, want /fetch/path/to/module", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	if err := c.FetchModule(context.Background(), "path/to/module"); err != nil {
		t.Fatal(err)
	}
}

func TestFetchWithVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch/path/to/module@v1.0.0" {
			t.Errorf("path = %q, want /fetch/path/to/module@v1.0.0", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	if err := c.FetchModule(context.Background(), "path/to/module@v1.0.0"); err != nil {
		t.Fatal(err)
	}
}

func TestFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `"path/to/module" could not be found.`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	err := c.FetchModule(context.Background(), "path/to/module")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if apiErr.Code != http.StatusNotFound {
		t.Errorf("Code = %d, want 404", apiErr.Code)
	}
	if want := `"path/to/module" could not be found.`; apiErr.Message != want {
		t.Errorf("Message = %q, want %q", apiErr.Message, want)
	}
}

func TestFetchErrorEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	err := c.FetchModule(context.Background(), "path/to/module")
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want HTTPError", err)
	}
	if httpErr != HTTPError(http.StatusInternalServerError) {
		t.Errorf("HTTPError = %d, want %d", httpErr, http.StatusInternalServerError)
	}
}

func pageServer(t *testing.T, path, limit string, resp any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %q, want %s", r.URL.Path, path)
		}
		checkQuery(t, r.URL.Query().Get("limit"), limit, "limit")
		json.NewEncoder(w).Encode(resp)
	}))
}

func checkQuery(t *testing.T, got, want, name string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}
