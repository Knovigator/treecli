package cmd

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultUpdateRepo = "Knovigator/treecli"
	githubAPIBaseURL  = "https://api.github.com"
	treecliBinaryName = "treecli"
	legacyBinaryName  = "treectl"
)

// CurrentVersion is replaced by the release workflow. Dev builds keep "dev".
var CurrentVersion = "dev"

var updateOptions struct {
	repo       string
	installDir string
	check      bool
	force      bool
	json       bool
}

type updateResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	Repo            string `json:"repo"`
	Asset           string `json:"asset,omitempty"`
	InstallPath     string `json:"install_path,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Updated         bool   `json:"updated"`
	CheckOnly       bool   `json:"check_only,omitempty"`
	Message         string `json:"message"`
}

type latestReleaseResponse struct {
	TagName string `json:"tag_name"`
}

var UpdateCmd = &cobra.Command{
	Use:   "update [version]",
	Short: "Update treecli to the latest release",
	Long: "Download a treecli GitHub Release archive, verify it against checksums.txt, " +
		"and replace the current CLI binary.\n\n" +
		"Without a version argument, treecli installs the latest published release. Pass a " +
		"specific tag such as v0.1.2 to install that release instead. Self-update currently " +
		"supports macOS and Linux release archives.",
	Example: "  treecli update\n" +
		"  treecli update --check\n" +
		"  treecli update v0.1.2\n" +
		"  treecli update --install-dir ~/.local/bin",
	Args: cobra.MaximumNArgs(1),
	RunE: runUpdate,
}

func init() {
	UpdateCmd.Flags().StringVar(&updateOptions.repo, "repo", defaultUpdateRepo, "GitHub repository to download releases from")
	UpdateCmd.Flags().StringVar(&updateOptions.installDir, "install-dir", "", "Install to this directory instead of replacing the current executable")
	UpdateCmd.Flags().BoolVar(&updateOptions.check, "check", false, "Check the latest release without installing it")
	UpdateCmd.Flags().BoolVar(&updateOptions.force, "force", false, "Reinstall even when the selected release matches the current version")
	UpdateCmd.Flags().BoolVar(&updateOptions.json, "json", false, "Print the update result as JSON")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	repo := strings.TrimSpace(updateOptions.repo)
	if repo == "" {
		return fmt.Errorf("--repo is required")
	}

	version := "latest"
	if len(args) == 1 {
		version = args[0]
	}

	client := &http.Client{Timeout: 60 * time.Second}
	tag, err := resolveUpdateTag(client, githubAPIBaseURL, repo, version)
	if err != nil {
		return err
	}

	assetName, err := releaseAssetName(tag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	installPath, err := updateInstallPath(updateOptions.installDir)
	if err != nil {
		return err
	}

	updateAvailable := !versionsEqual(CurrentVersion, tag)
	result := updateResult{
		CurrentVersion:  CurrentVersion,
		LatestVersion:   tag,
		Repo:            repo,
		Asset:           assetName,
		InstallPath:     installPath,
		UpdateAvailable: updateAvailable,
		CheckOnly:       updateOptions.check,
	}

	if updateOptions.check {
		if updateAvailable {
			result.Message = fmt.Sprintf("treecli %s is available", tag)
		} else {
			result.Message = fmt.Sprintf("treecli is already at %s", tag)
		}
		return printUpdateResult(result)
	}

	if !updateAvailable && !updateOptions.force {
		result.Message = fmt.Sprintf("treecli is already at %s", tag)
		return printUpdateResult(result)
	}

	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, tag)
	if err := downloadVerifyAndInstall(client, baseURL, assetName, installPath); err != nil {
		return err
	}

	result.Updated = true
	result.Message = fmt.Sprintf("Installed treecli %s to %s", tag, installPath)
	return printUpdateResult(result)
}

func resolveUpdateTag(client *http.Client, apiBaseURL string, repo string, version string) (string, error) {
	version = strings.TrimSpace(version)
	if version == "" || strings.EqualFold(version, "latest") {
		return fetchLatestReleaseTag(client, apiBaseURL, repo)
	}
	return normalizeReleaseTag(version), nil
}

func fetchLatestReleaseTag(client *http.Client, apiBaseURL string, repo string) (string, error) {
	url := strings.TrimRight(apiBaseURL, "/") + "/repos/" + strings.Trim(repo, "/") + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("accept", "application/vnd.github+json")
	req.Header.Set("user-agent", "treecli/"+CurrentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching latest release: status %d: %s", resp.StatusCode, readSmallResponse(resp.Body))
	}

	var out latestReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("parsing latest release: %w", err)
	}

	tag := normalizeReleaseTag(out.TagName)
	if tag == "" {
		return "", fmt.Errorf("latest release did not include a tag_name")
	}
	return tag, nil
}

func normalizeReleaseTag(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(version), "v") {
		return "v" + strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V")
	}
	return "v" + version
}

func releaseAssetName(tag string, goos string, goarch string) (string, error) {
	switch goos {
	case "darwin", "linux":
	default:
		return "", fmt.Errorf("self-update is not supported on %s; install manually from https://github.com/%s/releases", goos, defaultUpdateRepo)
	}

	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("self-update is not supported on %s/%s", goos, goarch)
	}

	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", treecliBinaryName, tag, goos, goarch), nil
}

func versionsEqual(current string, target string) bool {
	current = strings.TrimSpace(current)
	target = strings.TrimSpace(target)
	if current == "" || target == "" {
		return false
	}
	return strings.EqualFold(normalizeReleaseTag(current), normalizeReleaseTag(target))
}

func updateInstallPath(installDir string) (string, error) {
	if strings.TrimSpace(installDir) != "" {
		dir, err := expandHomeDir(strings.TrimSpace(installDir))
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, treecliBinaryName), nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving current executable: %w", err)
	}
	if evaluated, err := filepath.EvalSymlinks(executable); err == nil {
		executable = evaluated
	}
	return executable, nil
}

func expandHomeDir(path string) (string, error) {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home directory: %w", err)
		}
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func downloadVerifyAndInstall(client *http.Client, baseURL string, assetName string, installPath string) error {
	tmpdir, err := os.MkdirTemp("", "treecli-update-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpdir)

	assetPath := filepath.Join(tmpdir, assetName)
	checksumPath := filepath.Join(tmpdir, "checksums.txt")

	if err := downloadToFile(client, strings.TrimRight(baseURL, "/")+"/"+assetName, assetPath); err != nil {
		return err
	}
	if err := downloadToFile(client, strings.TrimRight(baseURL, "/")+"/checksums.txt", checksumPath); err != nil {
		return err
	}

	checksums, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("reading checksums: %w", err)
	}
	expected, err := checksumForAsset(string(checksums), assetName)
	if err != nil {
		return err
	}
	actual, err := sha256File(assetPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}

	extractedPath := filepath.Join(tmpdir, treecliBinaryName)
	if err := extractTreecliBinary(assetPath, extractedPath); err != nil {
		return err
	}

	if err := installExecutable(extractedPath, installPath); err != nil {
		return err
	}
	return nil
}

func downloadToFile(client *http.Client, url string, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("user-agent", "treecli/"+CurrentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", filepath.Base(path), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: status %d: %s", filepath.Base(path), resp.StatusCode, readSmallResponse(resp.Body))
	}

	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Base(path), err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}
	return nil
}

func checksumForAsset(checksums string, assetName string) (string, error) {
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[len(fields)-1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("could not find checksum for %s", assetName)
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", filepath.Base(path), err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hashing %s: %w", filepath.Base(path), err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func extractTreecliBinary(archivePath string, outputPath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("opening gzip archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		if header == nil || header.FileInfo().IsDir() {
			continue
		}
		if archiveBinaryName := filepath.Base(header.Name); archiveBinaryName != treecliBinaryName && archiveBinaryName != legacyBinaryName {
			continue
		}

		out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("creating extracted binary: %w", err)
		}
		if _, err := io.Copy(out, tarReader); err != nil {
			_ = out.Close()
			return fmt.Errorf("extracting binary: %w", err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("closing extracted binary: %w", err)
		}
		return os.Chmod(outputPath, 0755)
	}

	return fmt.Errorf("archive did not contain %s", treecliBinaryName)
}

func installExecutable(sourcePath string, targetPath string) error {
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating target dir: %w", err)
	}

	mode := os.FileMode(0755)
	if info, err := os.Stat(targetPath); err == nil {
		mode = info.Mode().Perm()
		if mode&0111 == 0 {
			mode |= 0111
		}
	}

	tmp, err := os.CreateTemp(targetDir, ".treecli-update-*")
	if err != nil {
		return fmt.Errorf("creating replacement file in %s: %w", targetDir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	source, err := os.Open(sourcePath)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("opening downloaded binary: %w", err)
	}
	if _, err := io.Copy(tmp, source); err != nil {
		_ = source.Close()
		_ = tmp.Close()
		return fmt.Errorf("writing replacement binary: %w", err)
	}
	if err := source.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("closing downloaded binary: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting replacement permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing replacement binary: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("installing %s: %w", targetPath, err)
	}
	return nil
}

func printUpdateResult(result updateResult) error {
	if updateOptions.json {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(string(encoded))
		return nil
	}

	fmt.Println(result.Message)
	return nil
}

func readSmallResponse(body io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
