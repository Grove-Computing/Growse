package supplychain

import (
	"strings"
	"testing"
)

const growseModule = "github.com/Grove-Computing/Growse"

func TestValidateSPDX(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			name: "module package",
			doc:  `{"spdxVersion":"SPDX-2.3","packages":[{"name":"github.com/Grove-Computing/Growse"}]}`,
		},
		{
			name: "module purl",
			doc:  `{"spdxVersion":"SPDX-2.3","packages":[{"name":"growse","externalRefs":[{"referenceLocator":"pkg:golang/github.com/Grove-Computing/Growse@v0.8.0"}]}]}`,
		},
		{
			name:    "invalid JSON",
			doc:     `{`,
			wantErr: "decode SPDX JSON",
		},
		{
			name:    "missing SPDX version",
			doc:     `{"packages":[{"name":"github.com/Grove-Computing/Growse"}]}`,
			wantErr: "missing or invalid SPDX version",
		},
		{
			name:    "empty packages",
			doc:     `{"spdxVersion":"SPDX-2.3","packages":[]}`,
			wantErr: "SBOM has no package components",
		},
		{
			name:    "missing module",
			doc:     `{"spdxVersion":"SPDX-2.3","packages":[{"name":"example.com/other"}]}`,
			wantErr: "SBOM does not contain Go module",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSPDX(strings.NewReader(tt.doc), growseModule)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateSPDX() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateSPDX() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
