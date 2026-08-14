package forms

import (
	"fmt"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func BenchmarkSerialize100FormControls(b *testing.B) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "benchmark"})
	if err := document.AppendChild(document.Root, form); err != nil {
		b.Fatal(err)
	}
	for index := 0; index < 100; index++ {
		control := document.CreateElement("input", map[string]string{
			"name": fmt.Sprintf("field_%03d", index), "value": fmt.Sprintf("value_%03d", index),
		})
		if err := document.AppendChild(form, control); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_ = EncodeURLEncoded(CollectEntries(document, form, nil))
	}
}
