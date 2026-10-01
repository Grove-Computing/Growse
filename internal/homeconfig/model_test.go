package homeconfig

import (
	"errors"
	"strings"
	"testing"
)

func TestShortcutCRUDNormalizesMergesAndReorders(t *testing.T) {
	settings := Defaults()
	if _, err := settings.Add("Docs", "https://Example.COM:443/docs/#part"); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Add("News", "https://news.example/"); err != nil {
		t.Fatal(err)
	}
	index, err := settings.Add("Updated docs", "https://example.com/docs")
	if err != nil {
		t.Fatal(err)
	}
	if index != 0 || len(settings.Shortcuts) != 2 || settings.Shortcuts[0].Title != "Updated docs" {
		t.Fatalf("duplicate merge = index %d shortcuts %#v", index, settings.Shortcuts)
	}
	if err := settings.Move(1, 0); err != nil {
		t.Fatal(err)
	}
	if settings.Shortcuts[0].Title != "News" {
		t.Fatalf("move = %#v", settings.Shortcuts)
	}
	if _, err := settings.Edit(1, "Merged", "https://news.example"); err != nil {
		t.Fatal(err)
	}
	if len(settings.Shortcuts) != 1 || settings.Shortcuts[0].Title != "Merged" {
		t.Fatalf("edit merge = %#v", settings.Shortcuts)
	}
	if err := settings.Delete(0); err != nil || len(settings.Shortcuts) != 0 {
		t.Fatalf("delete = %#v, %v", settings.Shortcuts, err)
	}
}

func TestShortcutValidationAndLimit(t *testing.T) {
	invalid := []Shortcut{
		{Title: "", URL: "https://example.com/"},
		{Title: strings.Repeat("界", MaxTitleScalars+1), URL: "https://example.com/"},
		{Title: "Credential", URL: "https://user:secret@example.com/"},
		{Title: "Script", URL: "javascript:alert(1)"},
		{Title: "Long", URL: "https://example.com/" + strings.Repeat("x", MaxURLBytes)},
	}
	for _, shortcut := range invalid {
		settings := Defaults()
		if _, err := settings.Add(shortcut.Title, shortcut.URL); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Add(%#v) error = %v", shortcut, err)
		}
	}
	settings := Defaults()
	for index := 0; index < MaxShortcuts; index++ {
		if _, err := settings.Add("item", "https://example.com/"+string(rune('a'+index))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := settings.Add("overflow", "https://overflow.example/"); !errors.Is(err, ErrLimit) {
		t.Fatalf("overflow error = %v", err)
	}
	if settings.Validate() != nil {
		t.Fatal("valid bounded settings rejected")
	}
}
