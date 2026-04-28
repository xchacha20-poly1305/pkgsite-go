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
		if r.URL.Path != "/v1/package/encoding/json" {
			t.Errorf("path = %q, want /v1/package/encoding/json", r.URL.Path)
		}
		if got := r.URL.Query().Get("version"); got != "go1.26.0" {
			t.Errorf("version = %q, want go1.26.0", got)
		}
		json.NewEncoder(w).Encode(Package{
			Path:              "encoding/json",
			ModulePath:        "std",
			ModuleVersion:     "go1.26.0",
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

func TestModule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/module/golang.org/x/text" {
			t.Errorf("path = %q, want /v1/module/golang.org/x/text", r.URL.Path)
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
	srv := pageServer(t, "/v1/versions/golang.org/x/text", "2", Page[Version]{
		Items: []Version{{Version: "v0.14.0"}, {Version: "v0.13.0"}},
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
	srv := pageServer(t, "/v1/vulns/golang.org/x/text", "100", Page[Vulnerability]{
		Items: []Vulnerability{{ID: "GO-2023-0001", Details: "A vulnerability."}},
		Total: 1,
	})
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Vulns(context.Background(), "golang.org/x/text", &ModuleOptions{Version: "v0.3.0"})
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
	srv := pageServer(t, "/v1/packages/golang.org/x/text", "100", Page[ModulePackage]{
		Items: []ModulePackage{
			{Path: "golang.org/x/text/language", Synopsis: "Package language implements BCP 47 language tags."},
		},
		Total: 1,
	})
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Packages(context.Background(), "golang.org/x/text", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Items[0].Path != "golang.org/x/text/language" {
		t.Errorf("Path = %q, want golang.org/x/text/language", resp.Items[0].Path)
	}
}

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkQuery(t, r.URL.Path, "/v1/search", "path")
		checkQuery(t, r.URL.Query().Get("q"), "json parser", "q")
		checkQuery(t, r.URL.Query().Get("symbol"), "Marshal", "symbol")
		json.NewEncoder(w).Encode(Page[SearchResult]{
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
	resp, err := c.Search(context.Background(), "json parser", &SearchOptions{Symbol: "Marshal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("len(Items) = %d, want 1", len(resp.Items))
	}
}

func TestSymbols(t *testing.T) {
	srv := pageServer(t, "/v1/symbols/encoding/json", "100", Page[Symbol]{
		Items: []Symbol{{
			Name:     "Marshal",
			Kind:     "func",
			Synopsis: "func Marshal(v any) ([]byte, error)",
		}},
		Total: 1,
	})
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.Symbols(context.Background(), "encoding/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Items[0].Name != "Marshal" {
		t.Errorf("Name = %q, want Marshal", resp.Items[0].Name)
	}
}

func TestImportedBy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ImportedBy{
			ModulePath: "std",
			Version:    "go1.26.0",
			ImportedBy: Page[string]{
				Items: []string{"github.com/foo/bar", "github.com/baz/qux"},
				Total: 2,
			},
		})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	resp, err := c.ImportedBy(context.Background(), "encoding/json", nil)
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
		json.NewEncoder(w).Encode(APIError{
			Code:    400,
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
		json.NewEncoder(w).Encode(APIError{Code: 404, Message: "not found"})
	}))
	defer srv.Close()

	c := NewClient(WithServer(srv.URL))
	_, err := c.Package(context.Background(), "nonexistent/pkg", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Code != 404 {
		t.Errorf("Code = %d, want 404", apiErr.Code)
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

func pageServer[T any](t *testing.T, path, limit string, resp Page[T]) *httptest.Server {
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
