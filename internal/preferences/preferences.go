package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Preferences struct {
	Version           int    "json:\"version\""
	OutputDir         string "json:\"output_dir\""
	AudioLanguages    string "json:\"audio_languages\""
	SubtitleLanguages string "json:\"subtitle_languages\""
	FilenameTemplate  string "json:\"filename_template\""
}

func Defaults() Preferences { return Preferences{Version: 1, OutputDir: "."} }
func Path() (string, error) {
	if p := os.Getenv("MAKEMKV_TUI_CONFIG"); p != "" {
		return p, nil
	}
	p, e := os.UserConfigDir()
	return filepath.Join(p, "makemkv-tui", "preferences.json"), e
}
func Load(path string) (Preferences, error) {
	p := Defaults()
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return p, nil
	}
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return p, fmt.Errorf("preferences %s: %w", path, e)
	}
	return p, p.Validate()
}
func (p Preferences) Validate() error {
	if p.Version != 1 {
		return fmt.Errorf("unsupported preferences version %d", p.Version)
	}
	for _, s := range []string{p.AudioLanguages, p.SubtitleLanguages} {
		for _, lang := range strings.Split(s, ",") {
			lang = strings.TrimSpace(lang)
			if lang != "" && !regexp.MustCompile("^[a-z]{3}$").MatchString(lang) {
				return fmt.Errorf("use comma-separated ISO 639-2 language codes, e.g. eng,jpn; blank keeps all")
			}
		}
	}
	if p.FilenameTemplate != "" {
		return ValidateName(strings.ReplaceAll(strings.ReplaceAll(p.FilenameTemplate, "{id}", "1"), "{name}", "Title"))
	}
	return nil
}
func ValidateName(s string) error {
	if s == "" || filepath.Base(s) != s || strings.ContainsAny(s, "/\\:\x00<>\"|?*") || strings.TrimSpace(s) != s || strings.HasSuffix(s, ".") || !strings.HasSuffix(strings.ToLower(s), ".mkv") {
		return fmt.Errorf("filename must be a plain .mkv basename without path separators or reserved characters")
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return fmt.Errorf("filename contains a control character")
		}
	}
	return nil
}
func Save(path string, p Preferences) error {
	if e := p.Validate(); e != nil {
		return e
	}
	dir := filepath.Dir(path)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".preferences-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	b, e := json.MarshalIndent(p, "", "  ")
	if e == nil {
		_, e = f.Write(append(b, '\n'))
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func Matches(list, lang string) bool {
	if strings.TrimSpace(list) == "" {
		return true
	}
	for _, v := range strings.Split(list, ",") {
		if strings.TrimSpace(v) == lang {
			return true
		}
	}
	return false
}
