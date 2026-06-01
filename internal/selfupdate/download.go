package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// binaryName is the name of the executable inside the release archive.
const binaryName = "specscore"

// defaultDownloadBaseURL is where GoReleaser publishes release assets.
const defaultDownloadBaseURL = "https://github.com/specscore/specscore-cli/releases/latest/download"

// AssetName builds the GoReleaser archive name for the given version and target
// OS/arch. The version's leading "v" is stripped to match GoReleaser's
// name_template ("specscore_{Version}_{Os}_{Arch}"). Windows uses .zip; all
// other platforms use .tar.gz.
func AssetName(version, goos, goarch string) string {
	v := normalize(version)
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("specscore_%s_%s_%s.%s", v, goos, goarch, ext)
}

// checksumsName builds the GoReleaser checksums filename for the given version.
func checksumsName(version string) string {
	return fmt.Sprintf("specscore_%s_checksums.txt", normalize(version))
}

// Downloader fetches and verifies release assets. BaseURL and Client are
// injectable so tests can target an httptest.Server.
type Downloader struct {
	// BaseURL is the directory under which assets and the checksums file live.
	// When empty, defaultDownloadBaseURL is used.
	BaseURL string
	// Client is the HTTP client used for requests. When nil, http.DefaultClient
	// is used.
	Client *http.Client
}

// DownloadAndVerify downloads the release asset matching the given version and
// target OS/arch, verifies its sha256 against the release checksums.txt, and —
// only on a successful match — extracts the specscore binary into a temp file
// whose path is returned. When goos/goarch are empty, runtime.GOOS/GOARCH are
// used.
//
// Verification happens before any extraction. On a checksum mismatch or a
// missing/unfetchable checksum entry, it returns a non-zero *exitcode.Error and
// an empty path, having touched no executable. This function never writes to or
// replaces the running executable; the swap is a separate concern.
func (d Downloader) DownloadAndVerify(ctx context.Context, version, goos, goarch string) (string, error) {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	asset := AssetName(version, goos, goarch)

	assetBytes, err := d.fetch(ctx, asset)
	if err != nil {
		return "", err
	}

	checksumBytes, err := d.fetch(ctx, checksumsName(version))
	if err != nil {
		return "", err
	}

	expected, err := findChecksum(checksumBytes, asset)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(assetBytes)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, expected) {
		return "", exitcode.InvalidStateErrorf(
			"checksum verification failed for %s: expected %s, got %s", asset, expected, actual)
	}

	// Verified — safe to extract.
	return extractBinary(assetBytes, goos)
}

// fetch GETs name relative to the downloader's base URL and returns the body.
func (d Downloader) fetch(ctx context.Context, name string) ([]byte, error) {
	base := d.BaseURL
	if base == "" {
		base = defaultDownloadBaseURL
	}
	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}

	url := strings.TrimRight(base, "/") + "/" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, exitcode.NotFoundErrorf("download of %s failed: status %d", name, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// findChecksum parses a GoReleaser checksums.txt (lines of "<sha256hex>  <file>")
// and returns the expected hex digest for asset. A missing entry is an error.
func findChecksum(checksums []byte, asset string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(checksums))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[1] == asset {
			return fields[0], nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", exitcode.NotFoundErrorf("no checksum entry found for %s", asset)
}

// extractBinary extracts the specscore executable from a verified archive into a
// temp file and returns its path. The archive format is chosen by goos
// (.zip for windows, .tar.gz otherwise).
func extractBinary(archive []byte, goos string) (string, error) {
	want := binaryName
	if goos == "windows" {
		want = binaryName + ".exe"
	}

	if goos == "windows" {
		return extractFromZip(archive, want)
	}
	return extractFromTarGz(archive, want)
}

func extractFromTarGz(archive []byte, want string) (string, error) {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", exitcode.UnexpectedErrorf("open gzip archive: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", exitcode.UnexpectedErrorf("read tar archive: %v", err)
		}
		if path.Base(hdr.Name) == want {
			return writeTempBinary(tr)
		}
	}
	return "", exitcode.NotFoundErrorf("binary %s not found in archive", want)
}

func extractFromZip(archive []byte, want string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return "", exitcode.UnexpectedErrorf("open zip archive: %v", err)
	}
	for _, f := range zr.File {
		if path.Base(f.Name) == want {
			rc, err := f.Open()
			if err != nil {
				return "", exitcode.UnexpectedErrorf("open %s in archive: %v", want, err)
			}
			defer rc.Close()
			return writeTempBinary(rc)
		}
	}
	return "", exitcode.NotFoundErrorf("binary %s not found in archive", want)
}

// writeTempBinary copies r into a new temp file and returns its path.
func writeTempBinary(r io.Reader) (string, error) {
	f, err := os.CreateTemp("", "specscore-update-*")
	if err != nil {
		return "", exitcode.UnexpectedErrorf("create temp file: %v", err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", exitcode.UnexpectedErrorf("write temp binary: %v", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", exitcode.UnexpectedErrorf("close temp binary: %v", err)
	}
	return f.Name(), nil
}
