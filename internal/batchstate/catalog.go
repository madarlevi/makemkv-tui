package batchstate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Directory deliberately does not depend on the source or output filesystem.
func Directory() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return stateDirectory(os.Getenv("XDG_STATE_HOME"), home)
}

func stateDirectory(xdg, home string) (string, error) {
	base := xdg
	if base == "" {
		if home == "" {
			return "", fmt.Errorf("cannot find home directory; set HOME, XDG_STATE_HOME, or --state PATH")
		}
		base = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("state directory must be absolute: %s; set XDG_STATE_HOME or --state PATH", base)
	}
	return filepath.Join(base, "makemkv-tui", "batches"), nil
}

// PlanPath is stable for a saved batch ID and descriptive without exposing a
// whole source path. An explicit path bypasses home/XDG resolution entirely.
func PlanPath(explicit string, s *State) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	dir, e := Directory()
	if e != nil {
		return "", e
	}
	if !ValidID(s.ID) {
		return "", fmt.Errorf("invalid batch ID")
	}
	var label strings.Builder
	for _, c := range filepath.Base(s.InputRoot) {
		if label.Len() >= 80 {
			break
		}
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_' {
			label.WriteRune(c)
		} else {
			label.WriteByte('-')
		}
	}
	name := strings.Trim(label.String(), "-")
	if name == "" {
		name = "batch"
	}
	return filepath.Join(dir, name+"-"+s.ID+".json"), nil
}

type Plan struct {
	Path  string
	State *State
	Err   error
}

// Plans reads atomic snapshots without taking a writer's lock. Invalid JSON is
// visible to callers, never silently discarded or replaced with a new plan.
func Plans(dir string) ([]Plan, error) {
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, fmt.Errorf("list batch plans %s: %w", dir, e)
	}
	var plans []Plan
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		p := Plan{Path: filepath.Join(dir, entry.Name())}
		info, e := entry.Info()
		if e != nil {
			p.Err = e
		} else if !info.Mode().IsRegular() {
			p.Err = fmt.Errorf("state must be a regular file: %s", p.Path)
		} else {
			f, e := os.Open(p.Path)
			if e != nil {
				p.Err = e
			} else {
				p.State, p.Err = decode(f, p.Path)
				f.Close()
			}
		}
		plans = append(plans, p)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Path < plans[j].Path })
	return plans, nil
}

func Matching(plans []Plan, state *State) (string, error) {
	for _, p := range plans {
		if p.Err != nil {
			return "", fmt.Errorf("cannot check existing plan %s: %w; repair it or use --new to explicitly create another plan", p.Path, p.Err)
		}
		if p.State.Status != Completed && samePath(p.State.InputRoot, state.InputRoot) && samePath(p.State.OutputRoot, state.OutputRoot) {
			return p.Path, nil
		}
	}
	return "", nil
}

func samePath(a, b string) bool {
	// Canonical paths already resolve aliases; Windows path case is insensitive.
	if os.PathSeparator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}
