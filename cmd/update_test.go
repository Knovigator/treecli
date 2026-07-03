package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveUpdateTagFetchesLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/Knovigator/treectl/releases/latest" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0"}`))
	}))
	t.Cleanup(server.Close)

	tag, err := resolveUpdateTag(server.Client(), server.URL, defaultUpdateRepo, "latest")
	if err != nil {
		t.Fatalf("resolveUpdateTag returned error: %v", err)
	}
	if tag != "v0.2.0" {
		t.Fatalf("expected v0.2.0, got %q", tag)
	}
}

func TestResolveUpdateTagNormalizesSpecificVersion(t *testing.T) {
	tag, err := resolveUpdateTag(http.DefaultClient, githubAPIBaseURL, defaultUpdateRepo, "0.2.0")
	if err != nil {
		t.Fatalf("resolveUpdateTag returned error: %v", err)
	}
	if tag != "v0.2.0" {
		t.Fatalf("expected v0.2.0, got %q", tag)
	}

	if got := normalizeReleaseTag("V0.2.0"); got != "v0.2.0" {
		t.Fatalf("expected uppercase V to normalize to v0.2.0, got %q", got)
	}
}

func TestReleaseAssetName(t *testing.T) {
	asset, err := releaseAssetName("v0.2.0", "darwin", "arm64")
	if err != nil {
		t.Fatalf("releaseAssetName returned error: %v", err)
	}
	if asset != "treectl_v0.2.0_darwin_arm64.tar.gz" {
		t.Fatalf("unexpected asset name %q", asset)
	}

	if _, err := releaseAssetName("v0.2.0", "windows", "amd64"); err == nil {
		t.Fatal("expected windows self-update to be unsupported")
	}
}

func TestChecksumForAsset(t *testing.T) {
	checksum, err := checksumForAsset("abc123  treectl_v0.2.0_linux_amd64.tar.gz\n", "treectl_v0.2.0_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("checksumForAsset returned error: %v", err)
	}
	if checksum != "abc123" {
		t.Fatalf("expected abc123, got %q", checksum)
	}
}

func TestVersionsEqualNormalizesVPrefix(t *testing.T) {
	if !versionsEqual("0.2.0", "v0.2.0") {
		t.Fatal("expected versions with and without v prefix to match")
	}
	if versionsEqual("dev", "v0.2.0") {
		t.Fatal("expected dev build not to match release tag")
	}
}

func TestUpdateInstallPathDoesNotCreateInstallDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")

	path, err := updateInstallPath(dir)
	if err != nil {
		t.Fatalf("updateInstallPath returned error: %v", err)
	}
	if path != filepath.Join(dir, "treectl") {
		t.Fatalf("unexpected install path %q", path)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected install dir not to be created, stat error: %v", err)
	}
}

func TestExtractTreectlBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "treectl.tar.gz")
	outputPath := filepath.Join(dir, "treectl")

	archive := buildTreectlArchive(t, []byte("new-binary"))
	if err := os.WriteFile(archivePath, archive, 0644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	if err := extractTreectlBinary(archivePath, outputPath); err != nil {
		t.Fatalf("extractTreectlBinary returned error: %v", err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("unexpected binary content %q", got)
	}
}

func TestDownloadVerifyAndInstall(t *testing.T) {
	assetName := "treectl_v0.2.0_linux_amd64.tar.gz"
	archive := buildTreectlArchive(t, []byte("new-binary"))
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release/" + assetName:
			_, _ = w.Write(archive)
		case "/release/checksums.txt":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	targetPath := filepath.Join(t.TempDir(), "bin", "treectl")
	if err := downloadVerifyAndInstall(server.Client(), server.URL+"/release", assetName, targetPath); err != nil {
		t.Fatalf("downloadVerifyAndInstall returned error: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("unexpected installed binary content %q", got)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatalf("expected installed binary to be executable, got mode %v", info.Mode().Perm())
	}
}

func buildTreectlArchive(t *testing.T, binary []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	header := &tar.Header{
		Name: "treectl",
		Mode: 0755,
		Size: int64(len(binary)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(binary); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	return buf.Bytes()
}
