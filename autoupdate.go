package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const githubRepo = "sillygru/gurtcli"

type updateCheckResult struct {
	latestVersion string
	needsUpdate   bool
	err           error
}

type updatePerformResult struct {
	err      error
	upToDate bool
}

func CheckLatestVersion(ctx context.Context) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 403, 429:
		return "", nil
	case 200:
	default:
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

func compareVersions(a, b string) int {
	parse := func(v string) []int {
		v = strings.TrimPrefix(v, "v")
		parts := strings.Split(v, ".")
		nums := make([]int, len(parts))
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil
			}
			nums[i] = n
		}
		return nums
	}

	va := parse(a)
	vb := parse(b)
	if va == nil || vb == nil {
		return 0
	}

	maxLen := len(va)
	if len(vb) > maxLen {
		maxLen = len(vb)
	}
	for i := 0; i < maxLen; i++ {
		var na, nb int
		if i < len(va) {
			na = va[i]
		}
		if i < len(vb) {
			nb = vb[i]
		}
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
	}
	return 0
}

func downloadRelease(ctx context.Context, version string) (string, error) {
	verStr := strings.TrimPrefix(version, "v")

	var archiveName string
	switch runtime.GOOS {
	case "windows":
		archiveName = fmt.Sprintf("gurtcli_%s_%s_%s.zip", verStr, runtime.GOOS, runtime.GOARCH)
	default:
		archiveName = fmt.Sprintf("gurtcli_%s_%s_%s.tar.gz", verStr, runtime.GOOS, runtime.GOARCH)
	}

	downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", githubRepo, version, archiveName)

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", archiveName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed: %s returned %d", downloadURL, resp.StatusCode)
	}

	tmpDir, err := os.MkdirTemp("", "gurtcli-update-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("self-update on Windows is not yet supported")
	}

	return extractTarGz(resp.Body, tmpDir)
}

func extractTarGz(r io.Reader, destDir string) (string, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}
		if hdr.Name != "gurtcli" {
			continue
		}

		outPath := filepath.Join(destDir, "gurtcli")
		outFile, err := os.Create(outPath)
		if err != nil {
			return "", fmt.Errorf("creating temp binary: %w", err)
		}
		defer outFile.Close()

		if _, err := io.Copy(outFile, tr); err != nil {
			return "", fmt.Errorf("extracting binary: %w", err)
		}

		if err := outFile.Chmod(0755); err != nil {
			return "", fmt.Errorf("chmod binary: %w", err)
		}

		return outPath, nil
	}

	return "", fmt.Errorf("binary not found in archive")
}

func swapBinary(tempPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving symlink: %w", err)
	}

	oldPath := execPath + ".old"
	if err := os.Rename(execPath, oldPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	if err := os.Rename(tempPath, execPath); err != nil {
		os.Rename(oldPath, execPath)
		return fmt.Errorf("replacing binary: %w", err)
	}

	if err := os.Chmod(execPath, 0755); err != nil {
		os.Rename(execPath, oldPath)
		os.Rename(oldPath, execPath)
		return fmt.Errorf("chmod binary: %w", err)
	}

	if err := syscall.Exec(execPath, os.Args, os.Environ()); err != nil {
		os.Rename(execPath, oldPath)
		os.Rename(oldPath, execPath)
		return fmt.Errorf("restarting: %w", err)
	}

	return nil
}

func cleanOldBinary() {
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return
	}
	os.Remove(execPath + ".old")
}

func checkForUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		latest, err := CheckLatestVersion(ctx)
		if err != nil || latest == "" {
			return updateCheckResult{err: err}
		}

		needsUpdate := compareVersions(strings.TrimPrefix(latest, "v"), Version) > 0

		return updateCheckResult{
			latestVersion: latest,
			needsUpdate:   needsUpdate,
		}
	}
}

func checkAndUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		latest, err := CheckLatestVersion(ctx)
		if err != nil {
			return updatePerformResult{err: fmt.Errorf("checking for updates: %w", err)}
		}
		if latest == "" {
			return updatePerformResult{err: fmt.Errorf("could not check for updates (rate limited?)")}
		}
		if compareVersions(strings.TrimPrefix(latest, "v"), Version) <= 0 {
			return updatePerformResult{upToDate: true}
		}

		fmt.Fprintf(os.Stderr, "\nDownloading gurtcli %s...\n", latest)

		tempPath, err := downloadRelease(ctx, latest)
		if err != nil {
			return updatePerformResult{err: err}
		}

		fmt.Fprintf(os.Stderr, "Installing update...\n")

		if err := swapBinary(tempPath); err != nil {
			return updatePerformResult{err: err}
		}

		return nil
	}
}

func performUpdateCmd(version string) tea.Cmd {
	return func() tea.Msg {
		fmt.Fprintf(os.Stderr, "\nDownloading gurtcli %s...\n", version)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		tempPath, err := downloadRelease(ctx, version)
		if err != nil {
			return updatePerformResult{err: err}
		}

		fmt.Fprintf(os.Stderr, "Installing update...\n")

		if err := swapBinary(tempPath); err != nil {
			return updatePerformResult{err: err}
		}

		return nil
	}
}
