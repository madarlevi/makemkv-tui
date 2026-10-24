// Package batchstate owns the versioned on-disk batch format and its invariants.
package batchstate

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

type Status string

const (
	Planning    Status = "planning"
	Ready       Status = "ready"
	Running     Status = "running"
	Stopping    Status = "stopping"
	Paused      Status = "paused"
	Completed   Status = "completed"
	Failed      Status = "failed"
	Pending     Status = "pending"
	Skipped     Status = "skipped"
	Interrupted Status = "interrupted"
)

type Failure struct {
	At      time.Time
	Message string
}
type Verification struct {
	Size            int64
	SHA256          string
	EBML            bool
	EngineSucceeded bool
	ValidatedAt     time.Time
}
type Title struct {
	ID                                int
	Metadata                          *media.Title
	Selected                          bool
	Output                            string
	EstimatedBytes                    int64
	Status                            Status
	Attempts, Retries                 int
	Errors                            []Failure
	StartedAt, UpdatedAt, CompletedAt time.Time
	AttemptID                         string
	Verification                      *Verification
	StagedFile                        string `json:",omitempty"` // Actual engine basename inside the persisted private attempt.
	EngineSucceeded                   bool   `json:",omitempty"` // Durable acknowledgement before hashing/publication.
	StagingInvalid                    bool   `json:",omitempty"` // Explicit retry may rerip; old data is retained.
}
type ISO struct {
	Attempts                          int
	RelativePath                      string
	Size                              int64
	ModTimeNS                         int64
	Reviewed                          bool
	Status                            Status
	Metadata                          map[int]string
	Titles                            []Title
	Preferences                       preferences.Preferences
	Errors                            []Failure
	Retries                           int
	StartedAt, UpdatedAt, CompletedAt time.Time
}
type StopRequest struct {
	Mode        string
	RequestedAt time.Time
	ISO         int
}
type Progress struct {
	Selected, Completed, Pending, Interrupted, Failed, Skipped int
	CurrentFraction, Overall                                   float64
}
type State struct {
	MinimumTitleLength                                        int // Schema 1 fixes scan/extraction filtering to zero for stable IDs.
	SchemaVersion                                             int
	ID, InputRoot, OutputRoot                                 string
	Status                                                    Status
	ISOs                                                      []ISO
	Preferences                                               preferences.Preferences
	Profile                                                   string
	ProfileSHA256                                             string
	CurrentISO, CurrentTitle                                  int
	PlanningISO                                               int
	Stop                                                      StopRequest
	Progress                                                  Progress
	Operation                                                 string
	CreatedAt, UpdatedAt, StartedAt, ConfirmedAt, CompletedAt time.Time
	ElapsedSeconds                                            float64
	Errors                                                    []Failure
}

func ID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func ValidID(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
func OutputDir(rel string) string {
	return filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(rel)), strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))))
}
func OutputPath(rel string, id int) string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(OutputDir(rel)), fmt.Sprintf("title%d.mkv", id)))
}
func StageDir(s *State, i int, t *Title) string {
	return filepath.ToSlash(filepath.Join(".makemkv-batch", s.ID, fmt.Sprintf("%d-%d-%s", i, t.ID, t.AttemptID)))
}
func Candidate(s *State, i int, t *Title) string {
	if t.StagedFile != "" {
		return filepath.ToSlash(filepath.Join(filepath.FromSlash(StageDir(s, i, t)), t.StagedFile))
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(StageDir(s, i, t)), fmt.Sprintf("title%d.mkv", t.ID)))
}
func Local(s string) bool {
	return s != "" && filepath.IsLocal(filepath.FromSlash(s)) && filepath.ToSlash(filepath.Clean(filepath.FromSlash(s))) == s && s != "." && !strings.ContainsRune(s, 0)
}
func (s *State) Recount() {
	p := Progress{CurrentFraction: s.Progress.CurrentFraction}
	active := false
	for _, iso := range s.ISOs {
		for _, t := range iso.Titles {
			if !t.Selected {
				p.Skipped++
				continue
			}
			p.Selected++
			if t.Status == Running {
				active = true
			}
			switch t.Status {
			case Completed:
				p.Completed++
			case Failed:
				p.Failed++
			case Interrupted:
				p.Interrupted++
			default:
				p.Pending++
			}
		}
	}
	if !active {
		p.CurrentFraction = 0
	}
	if p.Selected > 0 {
		p.Overall = (float64(p.Completed) + max(0, min(1, p.CurrentFraction))) / float64(p.Selected)
	} else if s.Status == Completed {
		p.Overall = 1
	}
	s.Progress = p
}
func (s *State) AllReviewed() bool {
	for _, j := range s.ISOs {
		if !j.Reviewed {
			return false
		}
	}
	return true
}
func (s *State) Validate() error {
	bad := func(f string, a ...any) error { return fmt.Errorf("invalid batch state: "+f, a...) }
	if s.SchemaVersion != SchemaVersion {
		return bad("schema %d is unsupported (expected %d); use a compatible application, do not reset the plan", s.SchemaVersion, SchemaVersion)
	}
	if s.MinimumTitleLength != 0 {
		return bad("schema 1 requires minimum title length zero for stable IDs")
	}
	if !ValidID(s.ID) || !filepath.IsAbs(s.InputRoot) || !filepath.IsAbs(s.OutputRoot) {
		return bad("invalid ID or non-absolute roots")
	}
	switch s.Status {
	case Planning, Ready, Running, Stopping, Paused, Completed, Failed:
	default:
		return bad("unknown batch status %q", s.Status)
	}
	if len(s.ISOs) == 0 {
		return bad("empty ISO list")
	}
	if s.CurrentISO < -1 || s.CurrentISO >= len(s.ISOs) || s.CurrentTitle < -1 || s.PlanningISO < 0 || s.PlanningISO >= len(s.ISOs) {
		return bad("invalid current/planning position")
	}
	if s.Stop.Mode != "" && s.Stop.Mode != "immediate" && s.Stop.Mode != "after_iso" {
		return bad("unknown stop mode")
	}
	if s.Stop.ISO < -1 || s.Stop.ISO >= len(s.ISOs) {
		return bad("invalid stop ISO position")
	}
	if !s.StartedAt.IsZero() && !s.AllReviewed() {
		return bad("started batch contains unreviewed sources")
	}
	if e := s.Preferences.Validate(); e != nil {
		return bad("preferences: %v", e)
	}
	if s.Profile != "" && (len(s.ProfileSHA256) != 64 || !filepath.IsAbs(s.Profile)) {
		return bad("invalid profile snapshot")
	}
	seen, outputs := map[string]bool{}, map[string]bool{}
	previous := ""
	for i := range s.ISOs {
		j := &s.ISOs[i]
		if j.Attempts < 0 || j.Retries < 0 {
			return bad("negative ISO attempt counters")
		}
		if !Local(j.RelativePath) || !strings.EqualFold(filepath.Ext(j.RelativePath), ".iso") || seen[j.RelativePath] || (i > 0 && j.RelativePath <= previous) || j.Size < 0 {
			return bad("unsafe, duplicate or unordered ISO path %q", j.RelativePath)
		}
		previous = j.RelativePath
		seen[j.RelativePath] = true
		if e := j.Preferences.Validate(); e != nil {
			return bad("ISO preferences: %v", e)
		}
		if !itemStatus(j.Status) {
			return bad("unknown ISO status %q", j.Status)
		}
		ids := map[int]bool{}
		last := -1
		for k := range j.Titles {
			t := &j.Titles[k]
			if t.ID < 0 || ids[t.ID] || t.ID <= last || t.Output != OutputPath(j.RelativePath, t.ID) || !Local(t.Output) {
				return bad("unsafe/duplicate title or output in %s", j.RelativePath)
			}
			ids[t.ID] = true
			last = t.ID
			key := strings.ToLower(t.Output)
			if outputs[key] {
				return bad("output collision at %s", t.Output)
			}
			outputs[key] = true
			if !itemStatus(t.Status) || (!t.Selected && t.Status != Skipped) || (t.Selected && t.Status == Skipped) {
				return bad("inconsistent title selection/status")
			}
			if t.Metadata == nil || t.Metadata.ID != t.ID {
				return bad("missing title metadata")
			}
			if t.Attempts < 0 || t.Retries < 0 || t.EstimatedBytes < 0 {
				return bad("negative title counters")
			}
			if t.AttemptID != "" && !ValidID(t.AttemptID) {
				return bad("invalid staging attempt")
			}
			if t.StagedFile != "" && (filepath.Base(t.StagedFile) != t.StagedFile || !Local(t.StagedFile) || !strings.EqualFold(filepath.Ext(t.StagedFile), ".mkv") || t.AttemptID == "") {
				return bad("unsafe staged output filename")
			}
			if t.EngineSucceeded && t.AttemptID == "" {
				return bad("engine success lacks staging attempt")
			}
			if t.Verification != nil {
				if !t.Selected {
					return bad("unselected title has an execution checkpoint")
				}
				v := t.Verification
				b, e := hex.DecodeString(v.SHA256)
				if v.Size < 5 || e != nil || len(b) != 32 || !v.EBML || !v.EngineSucceeded || v.ValidatedAt.IsZero() {
					return bad("invalid verification record")
				}
			}
			if t.Status == Completed && t.Verification == nil {
				return bad("completed title lacks verification")
			}
		}
	}
	if (s.Status == Ready || s.Status == Completed || s.Status == Running || s.Status == Stopping) && !s.AllReviewed() {
		return bad("execution status has unreviewed ISOs")
	}
	if s.Status == Completed {
		for _, j := range s.ISOs {
			for _, t := range j.Titles {
				if t.Selected && t.Status != Completed {
					return bad("completed batch contains unfinished title")
				}
			}
		}
	}
	return nil
}
func itemStatus(s Status) bool {
	switch s {
	case Pending, Running, Completed, Skipped, Interrupted, Failed:
		return true
	}
	return false
}
func SortTitles(t []Title) { sort.Slice(t, func(i, j int) bool { return t[i].ID < t[j].ID }) }
