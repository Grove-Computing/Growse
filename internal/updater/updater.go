// Package updater checks for signed Growse release artifacts and installs a
// verified executable update for the current platform.
package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL         = "https://api.github.com/repos/Grove-Computing/Growse/releases/latest"
	defaultReleaseBaseURL = "https://github.com/Grove-Computing/Growse/releases/download"
	maxReleaseResponse    = 1 << 20
	maxChecksumResponse   = 4 << 10
	maxArchiveBytes       = 256 << 20
)

// CurrentVersion is replaced with a release tag by scripts/package-gui.sh.
// Development builds intentionally never offer an update.
var CurrentVersion = "dev"

// Release describes a newer release offered to the UI.
type Release struct {
	Version string
}

// Options provides test seams while production callers use New.
type Options struct {
	CurrentVersion string
	APIURL         string
	ReleaseBaseURL string
	HTTPClient     *http.Client
	ExecutablePath string
	GOOS           string
	GOARCH         string
	PID            int
	Restart        func(executablePath, stagedPath string) error
}

// Manager checks and installs Growse releases.
type Manager struct {
	currentVersion string
	apiURL         string
	releaseBaseURL string
	client         *http.Client
	executablePath string
	goos           string
	goarch         string
	pid            int
	restart        func(executablePath, stagedPath string) error
}

// New returns a production updater. An unavailable executable path disables it
// safely by making Check return no update.
func New() *Manager {
	executablePath, _ := os.Executable()
	return NewWithOptions(Options{CurrentVersion: CurrentVersion, ExecutablePath: executablePath})
}

// NewWithOptions returns an updater configured for tests or production.
func NewWithOptions(options Options) *Manager {
	if options.CurrentVersion == "" {
		options.CurrentVersion = CurrentVersion
	}
	if options.APIURL == "" {
		options.APIURL = defaultAPIURL
	}
	if options.ReleaseBaseURL == "" {
		options.ReleaseBaseURL = defaultReleaseBaseURL
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if options.GOOS == "" {
		options.GOOS = runtime.GOOS
	}
	if options.GOARCH == "" {
		options.GOARCH = runtime.GOARCH
	}
	if options.PID == 0 {
		options.PID = os.Getpid()
	}
	manager := &Manager{
		currentVersion: options.CurrentVersion,
		apiURL:         options.APIURL, releaseBaseURL: strings.TrimRight(options.ReleaseBaseURL, "/"),
		client: options.HTTPClient, executablePath: options.ExecutablePath,
		goos: options.GOOS, goarch: options.GOARCH, pid: options.PID,
	}
	if options.Restart != nil {
		manager.restart = options.Restart
	} else {
		manager.restart = manager.restartAfterInstall
	}
	return manager
}

// Check reports a newer stable release. Development builds and unsupported
// platforms do not contact GitHub.
func (manager *Manager) Check(ctx context.Context) (Release, bool, error) {
	if !validVersion(manager.currentVersion) || manager.executablePath == "" {
		return Release{}, false, nil
	}
	if _, _, _, err := artifactFor(manager.currentVersion, manager.goos, manager.goarch); err != nil {
		return Release{}, false, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manager.apiURL, nil)
	if err != nil {
		return Release{}, false, fmt.Errorf("更新確認リクエストを作成: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Growse/"+manager.currentVersion)
	response, err := manager.client.Do(request) // #nosec G704 -- production endpoint is the fixed GitHub Releases API; tests inject a local endpoint.
	if err != nil {
		return Release{}, false, fmt.Errorf("最新版を確認: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, false, fmt.Errorf("最新版を確認: HTTP %d", response.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxReleaseResponse))
	if err := decoder.Decode(&payload); err != nil {
		return Release{}, false, fmt.Errorf("最新版を解析: %w", err)
	}
	if payload.Draft || !validVersion(payload.TagName) {
		return Release{}, false, errors.New("最新版のバージョンが不正です")
	}
	if compareVersions(payload.TagName, manager.currentVersion) <= 0 {
		return Release{}, false, nil
	}
	return Release{Version: payload.TagName}, true, nil
}

// Apply downloads, verifies, stages, and restarts into release.
func (manager *Manager) Apply(ctx context.Context, release Release) error {
	if !validVersion(release.Version) || compareVersions(release.Version, manager.currentVersion) <= 0 {
		return errors.New("更新対象のバージョンが不正です")
	}
	archiveName, executableName, archiveKind, err := artifactFor(release.Version, manager.goos, manager.goarch)
	if err != nil {
		return err
	}
	workDirectory, err := os.MkdirTemp("", "growse-update-*")
	if err != nil {
		return fmt.Errorf("更新用ディレクトリを作成: %w", err)
	}
	defer os.RemoveAll(workDirectory)

	archivePath := filepath.Join(workDirectory, archiveName)
	checksumURL := manager.releaseURL(release.Version, archiveName+".sha256")
	checksum, err := manager.downloadChecksum(ctx, checksumURL)
	if err != nil {
		return err
	}
	if err := manager.downloadFile(ctx, manager.releaseURL(release.Version, archiveName), archivePath, maxArchiveBytes); err != nil {
		return err
	}
	if err := verifyChecksum(archivePath, checksum); err != nil {
		return err
	}
	extractedPath := filepath.Join(workDirectory, "extracted-growse")
	if err := extractExecutable(archivePath, executableName, archiveKind, extractedPath); err != nil {
		return err
	}
	stagedPath, err := stageExecutable(manager.executablePath, extractedPath)
	if err != nil {
		return err
	}
	if err := manager.restart(manager.executablePath, stagedPath); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("更新を適用: %w", err)
	}
	return nil
}

func (manager *Manager) releaseURL(version, name string) string {
	return manager.releaseBaseURL + "/" + version + "/" + name
}

func (manager *Manager) downloadChecksum(ctx context.Context, rawURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("チェックサム取得リクエストを作成: %w", err)
	}
	response, err := manager.client.Do(request) // #nosec G704 -- URL is constructed from the fixed release base, validated version, and platform artifact name.
	if err != nil {
		return "", fmt.Errorf("チェックサムを取得: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("チェックサムを取得: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxChecksumResponse))
	if err != nil {
		return "", fmt.Errorf("チェックサムを読み取り: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", errors.New("チェックサム形式が不正です")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", errors.New("チェックサム形式が不正です")
	}
	return strings.ToLower(fields[0]), nil
}

func (manager *Manager) downloadFile(ctx context.Context, rawURL, destination string, limit int64) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("更新取得リクエストを作成: %w", err)
	}
	response, err := manager.client.Do(request) // #nosec G704 -- URL is constructed from the fixed release base and a validated artifact name.
	if err != nil {
		return fmt.Errorf("更新を取得: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("更新を取得: HTTP %d", response.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- destination is a filename inside Apply's private temporary directory.
	if err != nil {
		return fmt.Errorf("更新ファイルを作成: %w", err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, limit+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("更新を書き込み: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("更新ファイルを閉じる: %w", closeErr)
	}
	if written > limit {
		return errors.New("更新ファイルが大きすぎます")
	}
	return nil
}

func verifyChecksum(path, expected string) error {
	file, err := os.Open(path) // #nosec G304 -- path is a private temporary file created by Apply.
	if err != nil {
		return fmt.Errorf("更新ファイルを開く: %w", err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return fmt.Errorf("更新ファイルを検証: %w", err)
	}
	if hex.EncodeToString(digest.Sum(nil)) != expected {
		return errors.New("更新ファイルのSHA-256チェックサムが一致しません")
	}
	return nil
}

func artifactFor(version, goos, goarch string) (archiveName, executableName, archiveKind string, err error) {
	if !validVersion(version) {
		return "", "", "", errors.New("リリースバージョンが不正です")
	}
	switch {
	case goos == "linux" && goarch == "amd64":
		return "growse_" + version + "_linux_amd64.tar.gz", "growse", "tar.gz", nil
	case goos == "darwin" && (goarch == "amd64" || goarch == "arm64"):
		return "growse_" + version + "_macos_" + goarch + ".tar.gz", "Growse.app/Contents/MacOS/growse", "tar.gz", nil
	case goos == "windows" && goarch == "amd64":
		return "growse_" + version + "_windows_amd64.zip", "growse.exe", "zip", nil
	default:
		return "", "", "", fmt.Errorf("自動更新に未対応の環境です: %s/%s", goos, goarch)
	}
}

func extractExecutable(archivePath, wanted, kind, destination string) error {
	switch kind {
	case "tar.gz":
		return extractTarExecutable(archivePath, wanted, destination)
	case "zip":
		return extractZipExecutable(archivePath, wanted, destination)
	default:
		return errors.New("更新アーカイブ形式が不正です")
	}
}

func extractTarExecutable(archivePath, wanted, destination string) error {
	file, err := os.Open(archivePath) // #nosec G304 -- archivePath is the verified private download created by Apply.
	if err != nil {
		return fmt.Errorf("更新アーカイブを開く: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("更新アーカイブを展開: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("更新アーカイブを展開: %w", err)
		}
		if cleanArchiveName(header.Name) != wanted || !header.FileInfo().Mode().IsRegular() {
			continue
		}
		return writeExtractedExecutable(destination, reader)
	}
	return errors.New("更新アーカイブに実行ファイルがありません")
}

func extractZipExecutable(archivePath, wanted, destination string) error {
	reader, err := zip.OpenReader(archivePath) // #nosec G304 -- archivePath is the verified private download created by Apply.
	if err != nil {
		return fmt.Errorf("更新アーカイブを展開: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if cleanArchiveName(file.Name) != wanted || !file.Mode().IsRegular() {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("更新実行ファイルを開く: %w", err)
		}
		writeErr := writeExtractedExecutable(destination, source)
		closeErr := source.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	return errors.New("更新アーカイブに実行ファイルがありません")
}

func cleanArchiveName(name string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(name)), "./")
}

func writeExtractedExecutable(destination string, source io.Reader) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700) // #nosec G302,G304 -- destination is a private temporary executable and requires owner execute permission.
	if err != nil {
		return fmt.Errorf("更新実行ファイルを作成: %w", err)
	}
	_, copyErr := io.Copy(file, io.LimitReader(source, maxArchiveBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("更新実行ファイルを書き込み: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("更新実行ファイルを閉じる: %w", closeErr)
	}
	return nil
}

func stageExecutable(executablePath, source string) (string, error) {
	directory := filepath.Dir(executablePath)
	staged, err := os.CreateTemp(directory, ".growse-update-*")
	if err != nil {
		return "", fmt.Errorf("更新を配置: %w", err)
	}
	stagedPath := staged.Name()
	cleanup := true
	defer func() {
		_ = staged.Close()
		if cleanup {
			_ = os.Remove(stagedPath)
		}
	}()
	sourceFile, err := os.Open(source) // #nosec G304 -- source is the extracted verified executable in Apply's private directory.
	if err != nil {
		return "", fmt.Errorf("更新実行ファイルを開く: %w", err)
	}
	defer sourceFile.Close()
	if _, err := io.Copy(staged, sourceFile); err != nil {
		return "", fmt.Errorf("更新を配置: %w", err)
	}
	if err := staged.Chmod(0o755); err != nil {
		return "", fmt.Errorf("更新の実行権限を設定: %w", err)
	}
	if err := staged.Sync(); err != nil {
		return "", fmt.Errorf("更新を同期: %w", err)
	}
	if err := staged.Close(); err != nil {
		return "", fmt.Errorf("更新を閉じる: %w", err)
	}
	cleanup = false
	return stagedPath, nil
}

func (manager *Manager) restartAfterInstall(executablePath, stagedPath string) error {
	if manager.goos == "windows" {
		script := fmt.Sprintf(
			"Wait-Process -Id %d -ErrorAction SilentlyContinue; Move-Item -Force -LiteralPath %s -Destination %s; Start-Process -FilePath %s",
			manager.pid, powershellQuote(stagedPath), powershellQuote(executablePath), powershellQuote(executablePath),
		)
		command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script) // #nosec G204 -- executable and flags are fixed; paths are PowerShell single-quoted and escaped.
		return command.Start()
	}
	if manager.goos == "darwin" {
		return restartMacOS(executablePath, stagedPath)
	}
	if err := os.Rename(stagedPath, executablePath); err != nil {
		return err
	}
	return exec.Command(executablePath).Start() // #nosec G204 -- executes the just-verified binary at the current executable path without a shell.
}

func restartMacOS(executablePath, stagedPath string) error {
	bundle := macOSBundlePath(executablePath)
	if bundle == "" {
		return errors.New("macOS Application Bundleを特定できません")
	}
	placeholder, err := os.CreateTemp(filepath.Dir(executablePath), ".growse-backup-*")
	if err != nil {
		return err
	}
	backupPath := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	if err := os.Rename(executablePath, backupPath); err != nil {
		return err
	}
	rollback := func() {
		_ = os.Remove(executablePath)
		_ = os.Rename(backupPath, executablePath)
	}
	if err := os.Rename(stagedPath, executablePath); err != nil {
		rollback()
		return err
	}
	if err := exec.Command("codesign", "--force", "--deep", "--sign", "-", bundle).Run(); err != nil { // #nosec G204 -- fixed signing command targets only the current Growse application bundle.
		rollback()
		return fmt.Errorf("更新後のApplication Bundleを署名: %w", err)
	}
	if err := exec.Command("open", "-n", bundle).Start(); err != nil { // #nosec G204 -- fixed executable opens only the current Growse application bundle.
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func macOSBundlePath(executablePath string) string {
	marker := string(filepath.Separator) + "Contents" + string(filepath.Separator) + "MacOS" + string(filepath.Separator)
	index := strings.LastIndex(executablePath, marker)
	if index < 0 {
		return ""
	}
	return executablePath[:index]
}

type parsedVersion struct {
	major, minor, patch int64
	prerelease          string
}

func validVersion(value string) bool {
	_, ok := parseVersion(value)
	return ok
}

func parseVersion(value string) (parsedVersion, bool) {
	if !strings.HasPrefix(value, "v") {
		return parsedVersion{}, false
	}
	version := strings.TrimPrefix(value, "v")
	core := version
	prerelease := ""
	if index := strings.IndexByte(version, '-'); index >= 0 {
		core, prerelease = version[:index], version[index+1:]
		if prerelease == "" {
			return parsedVersion{}, false
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return parsedVersion{}, false
	}
	values := make([]int64, 3)
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return parsedVersion{}, false
		}
		parsed, err := strconv.ParseInt(part, 10, 64)
		if err != nil || parsed < 0 {
			return parsedVersion{}, false
		}
		values[index] = parsed
	}
	if prerelease != "" {
		for _, identifier := range strings.Split(prerelease, ".") {
			if identifier == "" {
				return parsedVersion{}, false
			}
			for _, character := range identifier {
				if (character < '0' || character > '9') && (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') && character != '-' {
					return parsedVersion{}, false
				}
			}
		}
	}
	return parsedVersion{major: values[0], minor: values[1], patch: values[2], prerelease: prerelease}, true
}

func compareVersions(left, right string) int {
	l, leftOK := parseVersion(left)
	r, rightOK := parseVersion(right)
	if !leftOK || !rightOK {
		return 0
	}
	for _, pair := range [][2]int64{{l.major, r.major}, {l.minor, r.minor}, {l.patch, r.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if l.prerelease == r.prerelease {
		return 0
	}
	if l.prerelease == "" {
		return 1
	}
	if r.prerelease == "" {
		return -1
	}
	return comparePrerelease(l.prerelease, r.prerelease)
}

func comparePrerelease(left, right string) int {
	leftParts, rightParts := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < len(leftParts) && index < len(rightParts); index++ {
		if leftParts[index] == rightParts[index] {
			continue
		}
		leftNumber, leftErr := strconv.ParseInt(leftParts[index], 10, 64)
		rightNumber, rightErr := strconv.ParseInt(rightParts[index], 10, 64)
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNumber < rightNumber {
				return -1
			}
			return 1
		case leftErr == nil:
			return -1
		case rightErr == nil:
			return 1
		case leftParts[index] < rightParts[index]:
			return -1
		default:
			return 1
		}
	}
	if len(leftParts) < len(rightParts) {
		return -1
	}
	return 1
}
