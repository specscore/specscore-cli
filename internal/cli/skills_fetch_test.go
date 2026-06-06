package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type tentry struct {
	name     string // path under the archive root
	typeflag byte
	body     string
}

func gzTar(t *testing.T, root string, entries []tentry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		hdr := &tar.Header{Name: root + "/" + e.name, Mode: 0o644, Typeflag: e.typeflag}
		if e.typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := io.WriteString(tw, e.body); err != nil {
				t.Fatalf("write body: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func marketplaceJSON(t *testing.T, repos ...string) []byte {
	t.Helper()
	var mf marketplaceManifest
	for _, r := range repos {
		var p struct {
			Name   string `json:"name"`
			Source struct {
				Repo string `json:"repo"`
			} `json:"source"`
		}
		p.Source.Repo = r
		mf.Plugins = append(mf.Plugins, p)
	}
	b, err := json.Marshal(mf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// startSkillsServer wires both base-URL env vars at a test server whose handler
// is supplied by the caller, and returns the server.
func startSkillsServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("SPECSCORE_MARKETPLACE_BASE_URL", srv.URL)
	t.Setenv("SPECSCORE_PLUGIN_ARCHIVE_BASE_URL", srv.URL)
	return srv
}

func TestFetchSkillBundle_HappyPath(t *testing.T) {
	tarball := gzTar(t, "plugin-main", []tentry{
		{"skills", tar.TypeDir, ""},
		{"skills/foo/SKILL.md", tar.TypeReg, "foo skill"},
		{"skills/foo/references/x.md", tar.TypeReg, "ref"},
		{"README.md", tar.TypeReg, "not a skill"},
	})
	startSkillsServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "marketplace.json"):
			_, _ = w.Write(marketplaceJSON(t, "test/plugin"))
		case strings.Contains(r.URL.Path, "/tar.gz/"):
			_, _ = w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	})

	bundle, err := fetchSkillBundle("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[string]string{}
	for _, f := range bundle {
		got[f.relPath] = string(f.content)
	}
	if got["foo/SKILL.md"] != "foo skill" || got["foo/references/x.md"] != "ref" {
		t.Errorf("unexpected bundle: %v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected only skills/ files, got %d: %v", len(got), got)
	}
}

func TestFetchSkillBundle_DedupAndEmptyRepo(t *testing.T) {
	tarball := gzTar(t, "plugin-main", []tentry{
		{"skills/foo/SKILL.md", tar.TypeReg, "foo"},
	})
	startSkillsServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "marketplace.json"):
			// two real repos + one empty-repo entry (must be skipped)
			_, _ = w.Write(marketplaceJSON(t, "test/p1", "", "test/p2"))
		case strings.Contains(r.URL.Path, "/tar.gz/"):
			_, _ = w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	})

	bundle, err := fetchSkillBundle("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bundle) != 1 || bundle[0].relPath != "foo/SKILL.md" {
		t.Errorf("expected deduped single file, got %v", bundle)
	}
}

func TestFetchSkillBundle_MarketplaceNotFound(t *testing.T) {
	startSkillsServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	if _, err := fetchSkillBundle("main"); err == nil || !strings.Contains(err.Error(), "marketplace manifest") {
		t.Errorf("expected marketplace manifest error, got %v", err)
	}
}

func TestFetchSkillBundle_MarketplaceInvalidJSON(t *testing.T) {
	startSkillsServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	if _, err := fetchSkillBundle("main"); err == nil || !strings.Contains(err.Error(), "parsing marketplace") {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestFetchSkillBundle_PluginTarballNotFound(t *testing.T) {
	startSkillsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "marketplace.json") {
			_, _ = w.Write(marketplaceJSON(t, "test/plugin"))
			return
		}
		http.NotFound(w, r)
	})
	if _, err := fetchSkillBundle("main"); err == nil || !strings.Contains(err.Error(), "fetching skills for test/plugin") {
		t.Errorf("expected plugin fetch error, got %v", err)
	}
}

func TestFetchSkillBundle_BadGzip(t *testing.T) {
	startSkillsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "marketplace.json") {
			_, _ = w.Write(marketplaceJSON(t, "test/plugin"))
			return
		}
		_, _ = w.Write([]byte("this is not gzip"))
	})
	if _, err := fetchSkillBundle("main"); err == nil {
		t.Error("expected gzip error")
	}
}

func TestHTTPGetBytes_NewRequestError(t *testing.T) {
	// A control character in the URL makes http.NewRequestWithContext fail.
	t.Setenv("SPECSCORE_MARKETPLACE_BASE_URL", "http://example.com/\x7f")
	if _, err := fetchSkillBundle("main"); err == nil {
		t.Error("expected NewRequest error")
	}
}

func TestHTTPGetBytes_DoError(t *testing.T) {
	old := skillsFetchTimeout
	t.Cleanup(func() { skillsFetchTimeout = old })
	// Port 1 on loopback refuses immediately — exercises the client.Do error path.
	t.Setenv("SPECSCORE_MARKETPLACE_BASE_URL", "http://127.0.0.1:1")
	if _, err := fetchSkillBundle("main"); err == nil {
		t.Error("expected Do error")
	}
}

func TestHTTPGetBytes_ReadAllError(t *testing.T) {
	startSkillsServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Promise more bytes than we send, then return → client ReadAll fails.
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	})
	if _, err := fetchSkillBundle("main"); err == nil {
		t.Error("expected ReadAll error")
	}
}

func TestExtractSkills_CorruptTar(t *testing.T) {
	// Valid gzip wrapping non-tar garbage → tar.Next returns a non-EOF error.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write(bytes.Repeat([]byte("garbage!"), 200))
	_ = gw.Close()
	if _, err := extractSkillsFromTarball(buf.Bytes()); err == nil {
		t.Error("expected corrupt-tar error")
	}
}

func TestExtractSkills_TruncatedFile(t *testing.T) {
	// Header claims 100 bytes but only 5 are written, and the tar is not closed,
	// so io.ReadAll on the entry body returns an unexpected-EOF error.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{
		Name: "r-main/skills/a/SKILL.md", Mode: 0o644, Size: 100, Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(tw, "short")
	_ = gw.Close() // deliberately skip tw.Close() to truncate the entry
	if _, err := extractSkillsFromTarball(buf.Bytes()); err == nil {
		t.Error("expected truncated-file error")
	}
}

func TestStripSkillsPrefix(t *testing.T) {
	cases := map[string]string{
		"plugin-main/skills/idea/SKILL.md": "idea/SKILL.md",
		"plugin-main/README.md":            "",
		"plugin-main/docs/x":               "",
		"plugin-main/skills/":              "",
		"justone":                          "",
	}
	for in, want := range cases {
		if got := stripSkillsPrefix(in); got != want {
			t.Errorf("stripSkillsPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveMarketplaceRef(t *testing.T) {
	t.Setenv("SPECSCORE_MARKETPLACE_REF", "from-env")
	if got := resolveMarketplaceRef("from-flag"); got != "from-flag" {
		t.Errorf("flag should win: got %q", got)
	}
	if got := resolveMarketplaceRef(" "); got != "from-env" {
		t.Errorf("blank flag should fall back to env: got %q", got)
	}
	t.Setenv("SPECSCORE_MARKETPLACE_REF", "")
	if got := resolveMarketplaceRef(""); got != "main" {
		t.Errorf("default should be main: got %q", got)
	}
}

func TestFetchSkillBundle_RefThreadedIntoURLs(t *testing.T) {
	tarball := gzTar(t, "plugin-main", []tentry{
		{"skills/foo/SKILL.md", tar.TypeReg, "foo"},
	})
	var sawManifestRef, sawArchiveRef bool
	startSkillsServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "marketplace.json"):
			if strings.Contains(r.URL.Path, "/v9.9.9/") {
				sawManifestRef = true
			}
			_, _ = w.Write(marketplaceJSON(t, "test/plugin"))
		case strings.Contains(r.URL.Path, "/tar.gz/"):
			if strings.HasSuffix(r.URL.Path, "/tar.gz/v9.9.9") {
				sawArchiveRef = true
			}
			_, _ = w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	})

	if _, err := fetchSkillBundle("v9.9.9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawManifestRef {
		t.Error("ref not used in marketplace manifest URL")
	}
	if !sawArchiveRef {
		t.Error("ref not used in plugin archive URL")
	}
}

func TestBaseURLDefaults(t *testing.T) {
	if got := marketplaceBaseURL(); got != "https://raw.githubusercontent.com" {
		t.Errorf("marketplace default = %q", got)
	}
	if got := pluginArchiveBaseURL(); got != "https://codeload.github.com" {
		t.Errorf("archive default = %q", got)
	}
	t.Setenv("SPECSCORE_MARKETPLACE_BASE_URL", "https://mirror.example")
	t.Setenv("SPECSCORE_PLUGIN_ARCHIVE_BASE_URL", "https://archive.example")
	if got := marketplaceBaseURL(); got != "https://mirror.example" {
		t.Errorf("marketplace override = %q", got)
	}
	if got := pluginArchiveBaseURL(); got != "https://archive.example" {
		t.Errorf("archive override = %q", got)
	}
}
