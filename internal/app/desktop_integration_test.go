package app

import (
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
