package app

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const linuxApplicationID = "io.github.grovecomputing.Growse"

func TestLinuxDesktopIntegrationAssets(t *testing.T) {
	assetDirectory := filepath.Join("..", "..", "packaging", "linux")
	desktopData, err := os.ReadFile(filepath.Join(assetDirectory, linuxApplicationID+".desktop"))
	if err != nil {
		t.Fatalf("read Desktop Entry: %v", err)
	}
	for _, required := range []string{
		"Type=Application",
		"Exec=growse",
		"Icon=" + linuxApplicationID,
		"Categories=Network;WebBrowser;",
		"StartupWMClass=" + linuxApplicationID,
	} {
		if !strings.Contains(string(desktopData), required) {
			t.Errorf("Desktop Entry does not contain %q", required)
		}
	}

	iconFile, err := os.Open(filepath.Join(assetDirectory, linuxApplicationID+".png"))
	if err != nil {
		t.Fatalf("open Desktop icon: %v", err)
	}
	defer iconFile.Close()
	configuration, err := png.DecodeConfig(iconFile)
	if err != nil {
		t.Fatalf("decode Desktop icon: %v", err)
	}
	if configuration.Width != 512 || configuration.Height != 512 {
		t.Fatalf("Desktop icon size = %dx%d, want 512x512", configuration.Width, configuration.Height)
	}
}

func TestMacOSApplicationBundleMetadata(t *testing.T) {
	metadata, err := os.ReadFile(filepath.Join("..", "..", "packaging", "macos", "Info.plist"))
	if err != nil {
		t.Fatalf("read macOS Info.plist: %v", err)
	}
	for _, required := range []string{
		"CFBundleExecutable",
		"CFBundleIconFile",
		"CFBundleIdentifier",
		linuxApplicationID,
		"@GROWSE_SHORT_VERSION@",
		"@GROWSE_BUILD_VERSION@",
	} {
		if !strings.Contains(string(metadata), required) {
			t.Errorf("macOS Info.plist does not contain %q", required)
		}
	}
}

func TestWindowsApplicationIntegrationAssets(t *testing.T) {
	assetDirectory := filepath.Join("..", "..", "packaging", "windows")
	iconData, err := os.ReadFile(filepath.Join(assetDirectory, "Growse.ico"))
	if err != nil {
		t.Fatalf("read Windows icon: %v", err)
	}
	if len(iconData) < 4 || !bytes.Equal(iconData[:4], []byte{0, 0, 1, 0}) {
		t.Fatal("Windows icon does not have ICO header")
	}
	shortcutData, err := os.ReadFile(filepath.Join(assetDirectory, "install-shortcut.ps1"))
	if err != nil {
		t.Fatalf("read Windows Shortcut Script: %v", err)
	}
	for _, required := range []string{
		`GetFolderPath("Programs")`,
		`CreateShortcut($shortcutPath)`,
		`$shortcut.TargetPath = $TargetPath`,
		`$shortcut.IconLocation = "$IconPath,0"`,
	} {
		if !strings.Contains(string(shortcutData), required) {
			t.Errorf("Windows Shortcut Script does not contain %q", required)
		}
	}
}
