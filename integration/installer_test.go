package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInstallerVerifiesAndRunsInstalledBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh supports macOS and Linux")
	}

	repoRoot := repositoryRoot(t)
	fixtureDir := t.TempDir()
	fakeBinDir := filepath.Join(t.TempDir(), "bin")
	installDir := filepath.Join(t.TempDir(), "installed")
	if err := os.MkdirAll(fakeBinDir, 0o755); err != nil {
		t.Fatalf("create fake bin: %v", err)
	}

	version := "v9.9.9-test"
	assetName := fmt.Sprintf("treectl_%s_%s_%s.tar.gz", version, runtime.GOOS, normalizedRuntimeArch(t))
	archive := installerArchive(t, "treectl", []byte("#!/bin/sh\nprintf 'fixture treecli works\\n'\n"))
	if err := os.WriteFile(filepath.Join(fixtureDir, assetName), archive, 0o644); err != nil {
		t.Fatalf("write release fixture: %v", err)
	}
	checksum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(checksum[:]), assetName)
	if err := os.WriteFile(filepath.Join(fixtureDir, "checksums.txt"), []byte(checksums), 0o644); err != nil {
		t.Fatalf("write checksum fixture: %v", err)
	}

	fakeCurl := `#!/bin/sh
set -eu
output=""
url=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o)
            output="$2"
            shift 2
            ;;
        -*)
            shift
            ;;
        *)
            url="$1"
            shift
            ;;
    esac
done
asset="${url##*/}"
source_path="${INSTALLER_FIXTURE_DIR}/${asset}"
if [ ! -f "$source_path" ]; then
    exit 22
fi
if [ -n "$output" ]; then
    cp "$source_path" "$output"
else
    command cat "$source_path"
fi
`
	if err := os.WriteFile(filepath.Join(fakeBinDir, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", filepath.Join(repoRoot, "install.sh"))
	cmd.Env = installerEnvironment(fakeBinDir, fixtureDir, installDir, version)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("installer failed: %v\n%s", err, output)
	}

	installedPath := filepath.Join(installDir, "treecli")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		installedPath = filepath.Join(installDir, "treectl")
	}
	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("stat installed binary: %v\n%s", err, output)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary is not executable: %v", info.Mode().Perm())
	}

	runOutput, err := exec.CommandContext(ctx, installedPath).CombinedOutput()
	if err != nil {
		t.Fatalf("installed binary did not run: %v\n%s", err, runOutput)
	}
	if strings.TrimSpace(string(runOutput)) != "fixture treecli works" {
		t.Fatalf("unexpected installed binary output: %q", runOutput)
	}

	t.Run("rejects a checksum mismatch", func(t *testing.T) {
		badChecksums := fmt.Sprintf("%064d  %s\n", 0, assetName)
		if err := os.WriteFile(filepath.Join(fixtureDir, "checksums.txt"), []byte(badChecksums), 0o644); err != nil {
			t.Fatalf("write bad checksum fixture: %v", err)
		}
		badInstallDir := filepath.Join(t.TempDir(), "installed")
		badContext, badCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer badCancel()
		badCommand := exec.CommandContext(badContext, "sh", filepath.Join(repoRoot, "install.sh"))
		badCommand.Env = installerEnvironment(fakeBinDir, fixtureDir, badInstallDir, version)
		badOutput, badErr := badCommand.CombinedOutput()
		if badErr == nil {
			t.Fatalf("expected checksum mismatch to fail\n%s", badOutput)
		}
		if !strings.Contains(string(badOutput), "checksum mismatch") {
			t.Fatalf("expected checksum error, got %q", badOutput)
		}
		for _, binaryName := range []string{"treecli", "treectl"} {
			if _, err := os.Stat(filepath.Join(badInstallDir, binaryName)); !os.IsNotExist(err) {
				t.Fatalf("checksum failure installed %s", binaryName)
			}
		}
	})
}

func installerArchive(t *testing.T, binaryName string, binary []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(binary))}); err != nil {
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
	return output.Bytes()
}

func installerEnvironment(fakeBinDir string, fixtureDir string, installDir string, version string) []string {
	environment := make([]string, 0, len(os.Environ())+9)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "PATH=") ||
			strings.HasPrefix(entry, "TREECTL_") ||
			strings.HasPrefix(entry, "TREECLI_") ||
			strings.HasPrefix(entry, "INSTALLER_FIXTURE_DIR=") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"PATH="+fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"INSTALLER_FIXTURE_DIR="+fixtureDir,
		"TREECTL_REPO=fixture/repo",
		"TREECLI_REPO=fixture/repo",
		"TREECTL_VERSION="+version,
		"TREECLI_VERSION="+version,
		"TREECTL_INSTALL_DIR="+installDir,
		"TREECLI_INSTALL_DIR="+installDir,
		"TREECLI_INSTALL_LEGACY=1",
	)
}

func normalizedRuntimeArch(t *testing.T) string {
	t.Helper()
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return runtime.GOARCH
	default:
		t.Skipf("installer does not support %s", runtime.GOARCH)
		return ""
	}
}
