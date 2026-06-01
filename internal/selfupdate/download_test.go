package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func TestAssetName(t *testing.T) {
	tests := []struct {
		name    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{"linux amd64", "1.2.3", "linux", "amd64", "specscore_1.2.3_linux_amd64.tar.gz"},
		{"darwin arm64", "1.2.3", "darwin", "arm64", "specscore_1.2.3_darwin_arm64.tar.gz"},
		{"windows amd64", "1.2.3", "windows", "amd64", "specscore_1.2.3_windows_amd64.zip"},
		{"strips leading v", "v1.2.3", "linux", "amd64", "specscore_1.2.3_linux_amd64.tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AssetName(tt.version, tt.goos, tt.goarch); got != tt.want {
				t.Errorf("AssetName(%q,%q,%q) = %q, want %q", tt.version, tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

// makeTarGz builds a .tar.gz archive containing a single file named binName
// with the given bytes.
func makeTarGz(t *testing.T, binName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{Name: binName, Mode: 0o755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, binName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(binName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// serveRelease starts an httptest.Server that serves the named asset and a
// checksums.txt with the given (possibly wrong) hash for the asset.
func serveRelease(t *testing.T, version, assetName string, assetBytes []byte, checksumHash string) (*httptest.Server, string) {
	t.Helper()
	checksumName := fmt.Sprintf("specscore_%s_checksums.txt", version)
	checksums := fmt.Sprintf("%s  %s\n", checksumHash, assetName)
	mux := http.NewServeMux()
	mux.HandleFunc("/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		w.Write(assetBytes)
	})
	mux.HandleFunc("/"+checksumName, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksums))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, srv.URL
}

func TestDownloadAndVerify_Success(t *testing.T) {
	version := "1.2.3"
	binContent := []byte("the new specscore binary bytes")
	assetName := AssetName(version, "linux", "amd64")
	archive := makeTarGz(t, "specscore", binContent)
	_, baseURL := serveRelease(t, version, assetName, archive, sha256Hex(archive))

	d := Downloader{BaseURL: baseURL, Client: http.DefaultClient}
	path, err := d.DownloadAndVerify(context.Background(), version, "linux", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if !bytes.Equal(got, binContent) {
		t.Errorf("extracted binary = %q, want %q", got, binContent)
	}
}

func TestDownloadAndVerify_Zip(t *testing.T) {
	version := "1.2.3"
	binContent := []byte("windows binary bytes")
	assetName := AssetName(version, "windows", "amd64")
	archive := makeZip(t, "specscore.exe", binContent)
	_, baseURL := serveRelease(t, version, assetName, archive, sha256Hex(archive))

	d := Downloader{BaseURL: baseURL, Client: http.DefaultClient}
	path, err := d.DownloadAndVerify(context.Background(), version, "windows", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if !bytes.Equal(got, binContent) {
		t.Errorf("extracted binary = %q, want %q", got, binContent)
	}
}

func TestDownloadAndVerify_ChecksumMismatchAborts(t *testing.T) {
	version := "1.2.3"
	binContent := []byte("the new specscore binary bytes")
	assetName := AssetName(version, "linux", "amd64")
	archive := makeTarGz(t, "specscore", binContent)
	// Wrong checksum: hash of unrelated bytes.
	wrongHash := sha256Hex([]byte("not the archive"))
	_, baseURL := serveRelease(t, version, assetName, archive, wrongHash)

	d := Downloader{BaseURL: baseURL, Client: http.DefaultClient}
	path, err := d.DownloadAndVerify(context.Background(), version, "linux", "amd64")
	if err == nil {
		t.Fatalf("expected verification error, got nil (path=%q)", path)
	}
	if path != "" {
		os.Remove(path)
		t.Fatalf("expected no extracted file on mismatch, got path %q", path)
	}

	var ec *exitcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitcode.Error, got %T: %v", err, err)
	}
	if ec.ExitCode() == 0 {
		t.Errorf("expected non-zero exit code, got 0")
	}
}

func TestDownloadAndVerify_MissingChecksumEntry(t *testing.T) {
	version := "1.2.3"
	binContent := []byte("the new specscore binary bytes")
	assetName := AssetName(version, "linux", "amd64")
	archive := makeTarGz(t, "specscore", binContent)
	checksumName := fmt.Sprintf("specscore_%s_checksums.txt", version)

	mux := http.NewServeMux()
	mux.HandleFunc("/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	mux.HandleFunc("/"+checksumName, func(w http.ResponseWriter, r *http.Request) {
		// Lists a different file, not our asset.
		fmt.Fprintf(w, "%s  specscore_%s_darwin_arm64.tar.gz\n", sha256Hex(archive), version)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	d := Downloader{BaseURL: srv.URL, Client: http.DefaultClient}
	path, err := d.DownloadAndVerify(context.Background(), version, "linux", "amd64")
	if err == nil {
		os.Remove(path)
		t.Fatalf("expected error for missing checksum entry, got nil")
	}
	if path != "" {
		os.Remove(path)
		t.Fatalf("expected no extracted file, got path %q", path)
	}
	var ec *exitcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitcode.Error, got %T: %v", err, err)
	}
}
