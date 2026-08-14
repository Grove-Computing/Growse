package supplychain

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ValidateSPDX verifies that an SPDX JSON document contains package components
// and identifies the expected Go module as one of those components.
func ValidateSPDX(r io.Reader, module string) error {
	var document struct {
		SPDXVersion string `json:"spdxVersion"`
		Packages    []struct {
			Name         string `json:"name"`
			ExternalRefs []struct {
				Locator string `json:"referenceLocator"`
			} `json:"externalRefs"`
		} `json:"packages"`
	}
	if err := json.NewDecoder(r).Decode(&document); err != nil {
		return fmt.Errorf("decode SPDX JSON: %w", err)
	}
	if !strings.HasPrefix(document.SPDXVersion, "SPDX-") {
		return fmt.Errorf("missing or invalid SPDX version")
	}
	if len(document.Packages) == 0 {
		return fmt.Errorf("SBOM has no package components")
	}
	for _, pkg := range document.Packages {
		if pkg.Name == module {
			return nil
		}
		for _, ref := range pkg.ExternalRefs {
			if strings.Contains(ref.Locator, module) {
				return nil
			}
		}
	}
	return fmt.Errorf("SBOM does not contain Go module %q", module)
}
