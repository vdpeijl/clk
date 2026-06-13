package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	if got := AssetName("linux", "amd64"); got != "clk_linux_amd64.tar.gz" {
		t.Errorf("AssetName(linux, amd64) = %q", got)
	}
	if got := AssetName("darwin", "arm64"); got != "clk_darwin_arm64.tar.gz" {
		t.Errorf("AssetName(darwin, arm64) = %q", got)
	}
}

func TestNeedsUpgrade(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"dev", "v1.0.0", true},
		{"", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"1.0.0", "v1.0.0", false}, // leading v ignored
		{"v1.0.0", "v1.1.0", true},
		{"v1.0.0", "", false}, // no release info
	}
	for _, c := range cases {
		if got := NeedsUpgrade(c.current, c.latest); got != c.want {
			t.Errorf("NeedsUpgrade(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestReleaseAssetURL(t *testing.T) {
	rel := Release{Assets: []Asset{
		{Name: "clk_linux_amd64.tar.gz", URL: "https://example/a"},
		{Name: "clk_darwin_arm64.tar.gz", URL: "https://example/b"},
	}}
	if got := rel.AssetURL("clk_darwin_arm64.tar.gz"); got != "https://example/b" {
		t.Errorf("AssetURL = %q", got)
	}
	if got := rel.AssetURL("missing"); got != "" {
		t.Errorf("AssetURL(missing) = %q, want empty", got)
	}
}

func TestExtractBinary(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"LICENSE": "license text",
		"clk":     "#!/binary\x00contents",
	})
	got, err := extractBinary(archive, "clk")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if string(got) != "#!/binary\x00contents" {
		t.Errorf("extracted %q", got)
	}

	if _, err := extractBinary(archive, "nope"); err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clk")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(path, []byte("new binary")); err != nil {
		t.Fatalf("replaceExecutable: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Errorf("file contents = %q", got)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("expected executable bit set, mode = %v", fi.Mode())
	}
}

func TestRunUpgrades(t *testing.T) {
	binContents := "fresh-clk-binary"
	asset := CurrentAssetName()
	archive := makeTarGz(t, map[string]string{"clk": binContents})

	mux := http.NewServeMux()
	mux.HandleFunc("/download/clk", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	var srv *httptest.Server
	mux.HandleFunc("/repos/vdpeijl/clk/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","assets":[{"name":"` + asset + `","browser_download_url":"` + srv.URL + `/download/clk"}]}`))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	execPath := filepath.Join(dir, "clk")
	if err := os.WriteFile(execPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := &Client{httpClient: srv.Client(), apiBaseURL: srv.URL, repo: "vdpeijl/clk"}
	res, err := c.Run(context.Background(), "v1.0.0", execPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Upgraded {
		t.Fatal("expected Upgraded=true")
	}
	if res.To != "v9.9.9" {
		t.Errorf("To = %q", res.To)
	}
	got, _ := os.ReadFile(execPath)
	if string(got) != binContents {
		t.Errorf("binary not replaced, got %q", got)
	}
}

func TestRunNoOpWhenCurrent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/vdpeijl/clk/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","assets":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), apiBaseURL: srv.URL, repo: "vdpeijl/clk"}
	res, err := c.Run(context.Background(), "v2.0.0", "/does/not/matter")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Upgraded {
		t.Error("expected no upgrade when already current")
	}
}

// makeTarGz builds an in-memory gzip-compressed tar from name->contents.
func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
