// Package batch implements planning and title-level execution independently of the TUI.
package batch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func canonicalDir(path string, create bool) (string, error) {
	a, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	if create {
		if e = os.MkdirAll(a, 0755); e != nil {
			return "", e
		}
	}
	p, e := batchstate.Canonical(a)
	if e != nil {
		return "", fmt.Errorf("resolve directory %s: %w", a, e)
	}
	st, e := os.Stat(p)
	if e != nil {
		return "", e
	}
	if !st.IsDir() {
		return "", fmt.Errorf("not a directory: %s", p)
	}
	return p, nil
}

// Discover never follows file or directory symlinks. Errors are reported rather than silently losing sources.
func Discover(input string) (string, []batchstate.ISO, error) {
	root, e := canonicalDir(input, false)
	if e != nil {
		return "", nil, e
	}
	jobs := []batchstate.ISO{}
	folders := map[string]string{}
	e = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".iso") {
			return nil
		}
		st, e := d.Info()
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if !batchstate.Local(rel) {
			return fmt.Errorf("unsafe source relative path %q", rel)
		}
		stem := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		if stem == "" || stem == "." || stem == ".." {
			return fmt.Errorf("ISO has no safe output directory name: %s", rel)
		}
		out := strings.ToLower(batchstate.OutputDir(rel))
		if previous, ok := folders[out]; ok {
			return fmt.Errorf("ISOs %q and %q map to the same output folder; rename one source", previous, rel)
		}
		folders[out] = rel
		jobs = append(jobs, batchstate.ISO{RelativePath: rel, Size: st.Size(), ModTimeNS: st.ModTime().UnixNano(), Status: batchstate.Pending, Preferences: preferences.Defaults()})
		return nil
	})
	if e != nil {
		return "", nil, e
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].RelativePath < jobs[j].RelativePath })
	return root, jobs, nil
}
func New(input, output string, p preferences.Preferences, profile string) (*batchstate.State, error) {
	root, jobs, e := Discover(input)
	if e != nil {
		return nil, e
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("no ISO files found recursively in %s", root)
	}
	out, e := canonicalDir(output, true)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	s := &batchstate.State{SchemaVersion: batchstate.SchemaVersion, ID: batchstate.ID(), InputRoot: root, OutputRoot: out, Status: batchstate.Planning, ISOs: jobs, Preferences: p, CurrentISO: -1, CurrentTitle: -1, CreatedAt: now, UpdatedAt: now}
	for i := range s.ISOs {
		s.ISOs[i].Preferences = p
	}
	if profile != "" {
		path, e := filepath.Abs(profile)
		if e != nil {
			return nil, e
		}
		path, e = batchstate.Canonical(path)
		if e != nil {
			return nil, e
		}
		hash, e := profileHash(path)
		if e != nil {
			return nil, e
		}
		s.Profile = path
		s.ProfileSHA256 = hash
	}
	return s, s.Validate()
}
func profileHash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("profile is not regular")
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func SetMetadata(s *batchstate.State, i int, d *media.Disc) error {
	if i < 0 || i >= len(s.ISOs) || d == nil {
		return fmt.Errorf("invalid ISO metadata position")
	}
	j := &s.ISOs[i]
	j.Metadata = d.Attributes
	j.Titles = nil
	for _, m := range d.Sorted() {
		n, _ := strconv.ParseInt(m.Attributes[11], 10, 64)
		j.Titles = append(j.Titles, batchstate.Title{ID: m.ID, Metadata: m, Output: batchstate.OutputPath(j.RelativePath, m.ID), EstimatedBytes: max(0, n), Status: batchstate.Skipped})
	}
	j.UpdatedAt = time.Now().UTC()
	return nil
}
func SetSelection(s *batchstate.State, i int, ids []int, p preferences.Preferences, reviewed bool) error {
	if !s.StartedAt.IsZero() {
		return fmt.Errorf("execution has started; selections are locked to preserve completed work")
	}
	if i < 0 || i >= len(s.ISOs) {
		return fmt.Errorf("invalid ISO index")
	}
	j := &s.ISOs[i]
	known := map[int]bool{}
	for _, t := range j.Titles {
		known[t.ID] = true
	}
	selected := map[int]bool{}
	for _, id := range ids {
		if !known[id] {
			return fmt.Errorf("unknown title ID %d", id)
		}
		selected[id] = true
	}
	if e := p.Validate(); e != nil {
		return e
	}
	now := time.Now().UTC()
	j.Preferences = p
	j.Reviewed = j.Reviewed || reviewed
	j.UpdatedAt = now
	for k := range j.Titles {
		t := &j.Titles[k]
		t.Selected = selected[t.ID]
		t.Status = batchstate.Skipped
		if t.Selected {
			t.Status = batchstate.Pending
		}
		t.UpdatedAt = now
	}
	j.Status = batchstate.Pending
	if len(ids) == 0 && j.Reviewed {
		j.Status = batchstate.Skipped
	}
	s.Status = batchstate.Planning
	if s.AllReviewed() {
		s.Status = batchstate.Ready
	}
	s.PlanningISO = i
	s.Recount()
	return nil
}
func Selected(j batchstate.ISO) []int {
	ids := []int{}
	for _, t := range j.Titles {
		if t.Selected {
			ids = append(ids, t.ID)
		}
	}
	return ids
}
func Estimate(s *batchstate.State) (int64, int) {
	var n int64
	unknown := 0
	for _, j := range s.ISOs {
		for _, t := range j.Titles {
			if t.Selected {
				n += t.EstimatedBytes
				if t.EstimatedBytes == 0 {
					unknown++
				}
			}
		}
	}
	return n, unknown
}
