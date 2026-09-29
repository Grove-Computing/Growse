package homeconfig

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/Grove-Computing/Growse/internal/omnibox"
)

const (
	MaxShortcuts    = 10
	MaxURLBytes     = 4 * 1024
	MaxTitleScalars = 128
)

var (
	ErrInvalid = errors.New("ホーム設定が不正です")
	ErrLimit   = errors.New("shortcutは10件までです")
	ErrIndex   = errors.New("shortcutが見つかりません")
)

type Shortcut struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Settings struct {
	Shortcuts  []Shortcut `json:"shortcuts,omitempty"`
	Background string     `json:"background,omitempty"`
}

func Defaults() Settings {
	return Settings{Background: "solid"}
}

func (settings Settings) Clone() Settings {
	settings.Shortcuts = append([]Shortcut(nil), settings.Shortcuts...)
	return settings
}

func (settings Settings) Validate() error {
	if len(settings.Shortcuts) > MaxShortcuts {
		return ErrLimit
	}
	seen := make(map[string]struct{}, len(settings.Shortcuts))
	for _, shortcut := range settings.Shortcuts {
		_, key, err := validateShortcut(shortcut)
		if err != nil {
			return err
		}
		if _, exists := seen[key]; exists {
			return ErrInvalid
		}
		seen[key] = struct{}{}
	}
	return nil
}

// Add inserts a shortcut or updates the existing normalized URL in place.
func (settings *Settings) Add(title, rawURL string) (int, error) {
	shortcut, key, err := validateShortcut(Shortcut{Title: title, URL: rawURL})
	if err != nil {
		return -1, err
	}
	for index, current := range settings.Shortcuts {
		_, currentKey, _ := validateShortcut(current)
		if currentKey == key {
			settings.Shortcuts[index] = shortcut
			return index, nil
		}
	}
	if len(settings.Shortcuts) >= MaxShortcuts {
		return -1, ErrLimit
	}
	settings.Shortcuts = append(settings.Shortcuts, shortcut)
	return len(settings.Shortcuts) - 1, nil
}

// Edit updates one shortcut. Editing to another shortcut's normalized URL
// merges the records and keeps the destination's position.
func (settings *Settings) Edit(index int, title, rawURL string) (int, error) {
	if index < 0 || index >= len(settings.Shortcuts) {
		return -1, ErrIndex
	}
	shortcut, key, err := validateShortcut(Shortcut{Title: title, URL: rawURL})
	if err != nil {
		return -1, err
	}
	for candidate, current := range settings.Shortcuts {
		if candidate == index {
			continue
		}
		_, currentKey, _ := validateShortcut(current)
		if currentKey != key {
			continue
		}
		settings.Shortcuts[candidate] = shortcut
		settings.Shortcuts = append(settings.Shortcuts[:index], settings.Shortcuts[index+1:]...)
		if index < candidate {
			candidate--
		}
		return candidate, nil
	}
	settings.Shortcuts[index] = shortcut
	return index, nil
}

func (settings *Settings) Delete(index int) error {
	if index < 0 || index >= len(settings.Shortcuts) {
		return ErrIndex
	}
	settings.Shortcuts = append(settings.Shortcuts[:index], settings.Shortcuts[index+1:]...)
	return nil
}

func (settings *Settings) Move(index, destination int) error {
	if index < 0 || index >= len(settings.Shortcuts) || destination < 0 || destination >= len(settings.Shortcuts) {
		return ErrIndex
	}
	if index == destination {
		return nil
	}
	moved := settings.Shortcuts[index]
	settings.Shortcuts = append(settings.Shortcuts[:index], settings.Shortcuts[index+1:]...)
	settings.Shortcuts = append(settings.Shortcuts, Shortcut{})
	copy(settings.Shortcuts[destination+1:], settings.Shortcuts[destination:])
	settings.Shortcuts[destination] = moved
	return nil
}

func validateShortcut(shortcut Shortcut) (Shortcut, string, error) {
	title := strings.TrimSpace(shortcut.Title)
	if title == "" || !utf8.ValidString(title) || utf8.RuneCountInString(title) > MaxTitleScalars {
		return Shortcut{}, "", ErrInvalid
	}
	rawURL := strings.TrimSpace(shortcut.URL)
	if rawURL == "" || len(rawURL) > MaxURLBytes || !utf8.ValidString(rawURL) {
		return Shortcut{}, "", ErrInvalid
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Hostname() == "" {
		return Shortcut{}, "", ErrInvalid
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return Shortcut{}, "", ErrInvalid
	}
	parsed.Scheme = scheme
	parsed.Fragment = ""
	parsed.RawFragment = ""
	stored := parsed.String()
	key, err := omnibox.NormalizeURL(stored)
	if err != nil {
		return Shortcut{}, "", ErrInvalid
	}
	return Shortcut{Title: title, URL: stored}, key, nil
}
