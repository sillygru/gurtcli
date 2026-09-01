package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
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

	tea "charm.land/bubbletea/v2"
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
	parse := func(v string) (nums []int, preRelease bool) {
		v = strings.TrimPrefix(v, "v")
		for _, p := range strings.Split(v, ".") {
			base := p
			if idx := strings.IndexByte(p, '-'); idx >= 0 {
				preRelease = true
				base = p[:idx]
			}
			n, err := strconv.Atoi(base)
			if err != nil {
				preRelease = true
				break
			}
			nums = append(nums, n)
			if preRelease {
				break
			}
		}
		return
	}

	va, aPre := parse(a)
	vb, bPre := parse(b)

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

	if aPre != bPre {
		if aPre {
			return -1
		}
		return 1
	}
	return 0
}

func downloadRelease(ctx context.Context, version string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("self-update on Windows is not yet supported")
	}

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

	tmpDir, err := createUpdateTempDir()
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	path, err := extractTarGz(resp.Body, tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	return path, nil
}

func createUpdateTempDir() (string, error) {
	if execPath, err := os.Executable(); err == nil {
		if execPath, err = filepath.EvalSymlinks(execPath); err == nil {
			dir := filepath.Dir(execPath)
			if dir != "" {
				if tmpDir, err := os.MkdirTemp(dir, ".gurtcli-update-*"); err == nil {
					return tmpDir, nil
				}
			}
		}
	}
	return os.MkdirTemp("", "gurtcli-update-*")
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
		if filepath.Base(hdr.Name) != "gurtcli" {
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

func isCrossDeviceError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid cross-device link") || strings.Contains(msg, "cross-device link")
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copying file: %w", err)
	}

	if err := out.Sync(); err != nil {
		out.Close()
		return fmt.Errorf("syncing file: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("closing destination: %w", err)
	}

	if err := os.Chmod(dst, perm); err != nil {
		return fmt.Errorf("chmod destination: %w", err)
	}

	return nil
}

func cleanupUpdateTempDir(tempPath string) {
	dir := filepath.Dir(tempPath)
	base := filepath.Base(dir)
	if strings.HasPrefix(base, "gurtcli-update-") || strings.HasPrefix(base, ".gurtcli-update-") {
		os.RemoveAll(dir)
	} else {
		os.Remove(tempPath)
	}
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
	// Ensure temp dir is cleaned even on failure (avoid leaking /tmp dirs).
	defer cleanupUpdateTempDir(tempPath)

	// Backup current binary. This is same-directory so Rename should succeed,
	// but handle cross-device defensively.
	backupViaRename := true
	if err := os.Rename(execPath, oldPath); err != nil {
		if !isCrossDeviceError(err) {
			return fmt.Errorf("backing up current binary: %w", err)
		}
		if err := copyFile(execPath, oldPath, 0755); err != nil {
			return fmt.Errorf("backing up current binary: %w", err)
		}
		backupViaRename = false
	}

	restore := func() {
		if backupViaRename {
			os.Rename(oldPath, execPath)
		} else {
			// Backup was a copy, so oldPath and execPath coexist. Restore via copy
			// if Rename fails (should be same device, but be defensive).
			if err := os.Rename(oldPath, execPath); err != nil {
				copyFile(oldPath, execPath, 0755)
			}
		}
	}

	// Move new binary into place, handling cross-device (e.g. /tmp vs /home).
	if err := os.Rename(tempPath, execPath); err != nil {
		if !isCrossDeviceError(err) {
			restore()
			return fmt.Errorf("replacing binary: %w", err)
		}
		// Cross-device: copy to a staging file in the same dir then atomically rename.
		staging := execPath + ".new"
		os.Remove(staging)
		if err := copyFile(tempPath, staging, 0755); err != nil {
			os.Remove(staging)
			restore()
			return fmt.Errorf("replacing binary: %w", err)
		}
		if err := os.Rename(staging, execPath); err != nil {
			os.Remove(staging)
			restore()
			return fmt.Errorf("replacing binary: %w", err)
		}
	}

	if err := os.Chmod(execPath, 0755); err != nil {
		restore()
		return fmt.Errorf("chmod binary: %w", err)
	}

	if err := syscall.Exec(execPath, os.Args, os.Environ()); err != nil {
		restore()
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
	os.Remove(execPath + ".new")
	dir := filepath.Dir(execPath)
	for _, pat := range []string{".gurtcli-update-*", "gurtcli-update-*"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pat))
		for _, m := range matches {
			os.RemoveAll(m)
		}
	}
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

func checkVersionCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		latest, err := CheckLatestVersion(ctx)
		if err != nil || latest == "" {
			return versionCheckResult{err: err}
		}

		needsUpdate := compareVersions(strings.TrimPrefix(latest, "v"), Version) > 0

		return versionCheckResult{
			latestVersion: latest,
			needsUpdate:   needsUpdate,
		}
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
