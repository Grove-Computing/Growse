package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckReportsOnlyNewerRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/latest" {
			http.NotFound(response, request)
			return
		}
		_, _ = io.WriteString(response, `{"tag_name":"v0.12.0","draft":false}`)
	}))
	defer server.Close()

	manager := NewWithOptions(Options{
		CurrentVersion: "v0.11.0", APIURL: server.URL + "/latest",
		ExecutablePath: filepath.Join(t.TempDir(), "growse"), GOOS: "linux", GOARCH: "amd64",
	})
	release, available, err := manager.Check(context.Background())
	if err != nil || !available || release.Version != "v0.12.0" {
		t.Fatalf("Check() = %#v, %v, %v", release, available, err)
	}

	manager.currentVersion = "v0.12.0"
	if release, available, err = manager.Check(context.Background()); err != nil || available || release != (Release{}) {
		t.Fatalf("current Check() = %#v, %v, %v", release, available, err)
	}
}

func TestDevelopmentBuildDoesNotContactReleaseAPI(t *testing.T) {
	contacted := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { contacted = true }))
	defer server.Close()
	manager := NewWithOptions(Options{
		CurrentVersion: "dev", APIURL: server.URL,
		ExecutablePath: filepath.Join(t.TempDir(), "growse"), GOOS: "linux", GOARCH: "amd64",
	})
	if _, available, err := manager.Check(context.Background()); err != nil || available || contacted {
		t.Fatalf("Check() available=%v err=%v contacted=%v", available, err, contacted)
	}
}

func TestApplyVerifiesAndStagesReleaseExecutable(t *testing.T) {
	const version = "v0.12.0"
	archive := linuxArchive(t, "new-growse-binary")
	digest := sha256.Sum256(archive)
	archiveName, _, _, err := artifactFor(version, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/" + version + "/" + archiveName:
			_, _ = response.Write(archive)
		case "/" + version + "/" + archiveName + ".sha256":
			_, _ = fmt.Fprintf(response, "%x  %s\n", digest, archiveName)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	directory := t.TempDir()
	executablePath := filepath.Join(directory, "growse")
	if err := os.WriteFile(executablePath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	restarted := false
	manager := NewWithOptions(Options{
		CurrentVersion: "v0.11.0", ReleaseBaseURL: server.URL,
		ExecutablePath: executablePath, GOOS: "linux", GOARCH: "amd64",
		Restart: func(gotExecutable, stagedPath string) error {
			restarted = true
			if gotExecutable != executablePath {
				t.Fatalf("executable = %q", gotExecutable)
			}
			content, err := os.ReadFile(stagedPath)
			if err != nil {
				return err
			}
			if string(content) != "new-growse-binary" {
				t.Fatalf("staged content = %q", content)
			}
			return os.Remove(stagedPath)
		},
	})
	if err := manager.Apply(context.Background(), Release{Version: version}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !restarted {
		t.Fatal("restart was not requested")
	}
}

func TestApplyRejectsChecksumMismatch(t *testing.T) {
	const version = "v0.12.0"
	archive := linuxArchive(t, "new-growse-binary")
	archiveName, _, _, _ := artifactFor(version, "linux", "amd64")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/"+version+"/"+archiveName+".sha256" {
			_, _ = io.WriteString(response, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  archive\n")
			return
		}
		_, _ = response.Write(archive)
	}))
	defer server.Close()
	manager := NewWithOptions(Options{
		CurrentVersion: "v0.11.0", ReleaseBaseURL: server.URL,
		ExecutablePath: filepath.Join(t.TempDir(), "growse"), GOOS: "linux", GOARCH: "amd64",
		Restart: func(string, string) error { t.Fatal("restart must not be called"); return nil },
	})
	if err := manager.Apply(context.Background(), Release{Version: version}); err == nil {
		t.Fatal("Apply() accepted mismatched checksum")
	}
}

func TestCompareVersions(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"v0.12.0", "v0.11.9", 1},
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0-rc.2", "v1.0.0-rc.10", -1},
		{"v1.0.0", "v1.0.0-rc.1", 1},
	} {
		if got := compareVersions(test.left, test.right); got != test.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func linuxArchive(t *testing.T, executable string) []byte {
	t.Helper()
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "growse", Mode: 0o755, Size: int64(len(executable))}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(tarWriter, executable); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
