package batch

import (
	"context"
	"errors"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/publish"
	"os"
	"path/filepath"
	"time"
)

type Saver interface{ Save(*batchstate.State) error }

func addError(s *batchstate.State, j *batchstate.ISO, t *batchstate.Title, e error) {
	f := batchstate.Failure{At: time.Now().UTC(), Message: e.Error()}
	s.Errors = append(s.Errors, f)
	if j != nil {
		j.Errors = append(j.Errors, f)
	}
	if t != nil {
		t.Errors = append(t.Errors, f)
	}
}
func isoStatus(j *batchstate.ISO) {
	if !j.Reviewed {
		return
	}
	selected, complete, failed, interrupted := 0, 0, false, false
	for _, t := range j.Titles {
		if !t.Selected {
			continue
		}
		selected++
		switch t.Status {
		case batchstate.Completed:
			complete++
		case batchstate.Failed:
			failed = true
		case batchstate.Interrupted:
			interrupted = true
		}
	}
	switch {
	case selected == 0:
		j.Status = batchstate.Skipped
	case complete == selected:
		j.Status = batchstate.Completed
		if j.CompletedAt.IsZero() {
			j.CompletedAt = time.Now().UTC()
		}
	case failed:
		j.Status = batchstate.Failed
	case interrupted:
		j.Status = batchstate.Interrupted
	default:
		j.Status = batchstate.Pending
	}
}

// Recover never adopts an uncheckpointed file, even if its name and signature look valid.
func Recover(ctx context.Context, s *batchstate.State, store Saver) error {
	return recoverUsing(ctx, s, store, publish.Publisher{})
}

func recoverUsing(ctx context.Context, s *batchstate.State, store Saver, p publish.Publisher) error {
	if e := s.Validate(); e != nil {
		return e
	}
	if e := ValidateSources(s); e != nil {
		return e
	}
	r, e := openOutput(s)
	if e != nil {
		return e
	}
	defer r.Close()
	if s.Status == batchstate.Completed {
		s.Status = batchstate.Paused
	}
	failed := false
	for i := range s.ISOs {
		j := &s.ISOs[i]
		for k := range j.Titles {
			if e = ctx.Err(); e != nil {
				return e
			}
			t := &j.Titles[k]
			if !t.Selected {
				continue
			}
			fail := func(err error, invalid bool) {
				t.Status = batchstate.Failed
				s.Status = batchstate.Failed
				t.StagingInvalid = invalid
				failed = true
				addError(s, j, t, fmt.Errorf("%w; staging retained at %s", err, filepath.Join(s.OutputRoot, filepath.FromSlash(batchstate.StageDir(s, i, t)))))
			}
			if e = checkPath(r, t.Output, true); e != nil {
				fail(e, false)
				continue
			}
			// A receipt survives a crash between successful MakeMKV completion
			// and the JSON checkpoint, including during the subsequent hash.
			if t.AttemptID != "" && !t.EngineSucceeded && t.Verification == nil {
				stage := filepath.Join(s.OutputRoot, filepath.FromSlash(batchstate.StageDir(s, i, t)))
				if e = checkPath(r, batchstate.StageDir(s, i, t), true); e != nil {
					fail(e, true)
					continue
				}
				receipt, err := backend.ReadReceipt(stage, source(s.InputRoot, j.RelativePath), t.ID)
				if err != nil {
					fail(err, os.IsNotExist(err) || errors.Is(err, errInvalidOutput))
					continue
				}
				if receipt != nil {
					t.StagedFile = receipt.File
					t.EngineSucceeded = true
					t.StagingInvalid = false
					if e = store.Save(s); e != nil {
						return e
					}
				}
			}
			_, statErr := r.Lstat(filepath.FromSlash(t.Output))
			exists := statErr == nil
			if statErr != nil && !os.IsNotExist(statErr) {
				fail(statErr, false)
				continue
			}
			if t.Verification == nil && t.EngineSucceeded {
				v, err := verify(ctx, r, batchstate.Candidate(s, i, t), true)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err != nil {
					fail(err, os.IsNotExist(err) || errors.Is(err, errInvalidOutput))
					continue
				}
				t.Verification = v
				t.StagingInvalid = false
				if e = store.Save(s); e != nil {
					return e
				}
			}
			switch {
			case exists:
				if t.Verification == nil {
					fail(fmt.Errorf("unverified output collision: %s; move the conflicting file before retrying", t.Output), false)
					break
				}
				v, err := verify(ctx, r, t.Output, true)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err != nil || !matches(t.Verification, v) {
					if err == nil {
						err = fmt.Errorf("output collision or verification mismatch: %s; an incomplete exclusive-copy destination is never overwritten; move it aside before resuming", t.Output)
					}
					fail(err, false)
					break
				}
				if err = syncOutputDir(r, t.Output); err != nil {
					fail(err, false)
					break
				}
				// Final file is valid; persist adoption BEFORE any staging cleanup.
				t.Status = batchstate.Completed
				t.StagingInvalid = false
				if t.CompletedAt.IsZero() {
					t.CompletedAt = time.Now().UTC()
				}
				if e = store.Save(s); e != nil {
					return e
				}
				if e = cleanup(s, r, i, t); e != nil {
					addError(s, j, t, fmt.Errorf("completed output retained; staging cleanup warning: %w", e))
				}
			case t.Verification != nil && t.AttemptID != "":
				v, err := verify(ctx, r, batchstate.Candidate(s, i, t), true)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err != nil || !matches(t.Verification, v) {
					invalid := err == nil || os.IsNotExist(err) || errors.Is(err, errInvalidOutput)
					if err == nil {
						err = fmt.Errorf("verified staging file changed: %s", batchstate.Candidate(s, i, t))
					}
					fail(err, invalid)
					break
				}
				if err = publishOutput(ctx, p, s, r, i, t); err != nil {
					if ctx.Err() != nil {
						t.Status = batchstate.Interrupted
						j.Status = batchstate.Interrupted
						s.Status = batchstate.Paused
						s.Stop = batchstate.StopRequest{Mode: "immediate", ISO: i, RequestedAt: time.Now().UTC()}
						addError(s, j, t, err)
						if e = store.Save(s); e != nil {
							return errors.Join(err, e)
						}
						return ctx.Err()
					}
					fail(err, false)
					break
				}
				t.Status = batchstate.Completed
				t.CompletedAt = time.Now().UTC()
				t.StagingInvalid = false
				s.Progress.CurrentFraction = 0
				if e = store.Save(s); e != nil {
					return e
				}
				if e = cleanup(s, r, i, t); e != nil {
					addError(s, j, t, fmt.Errorf("completed output retained; staging cleanup warning: %w", e))
				}
			case t.Status == batchstate.Completed:
				fail(fmt.Errorf("previously completed output is missing: %s; restore it or explicitly retry", t.Output), true)
			default:
				if t.Status == batchstate.Running {
					t.Status = batchstate.Interrupted
				}
				// Unacknowledged attempts (including interrupted partials) are
				// isolated and retained, never recursively erased. A new attempt
				// gets a different ID. No file is adopted solely because it is MKV.
			}
			if t.Status == batchstate.Failed {
				failed = true
			}
			t.UpdatedAt = time.Now().UTC()
		}
		isoStatus(j)
	}
	s.Progress.CurrentFraction = 0
	s.CurrentTitle = -1
	switch {
	case failed:
		s.Status = batchstate.Failed
	case !s.StartedAt.IsZero():
		s.Status = batchstate.Completed
		for _, j := range s.ISOs {
			for _, t := range j.Titles {
				if t.Selected && t.Status != batchstate.Completed {
					s.Status = batchstate.Paused
				}
			}
		}
		if s.Status == batchstate.Completed && s.CompletedAt.IsZero() {
			s.CompletedAt = time.Now().UTC()
		}
	case s.AllReviewed():
		s.Status = batchstate.Ready
	default:
		s.Status = batchstate.Planning
	}
	s.Recount()
	return store.Save(s)
}
func RetryFailed(s *batchstate.State, store Saver) error {
	changed := false
	for i := range s.ISOs {
		j := &s.ISOs[i]
		for k := range j.Titles {
			t := &j.Titles[k]
			if t.Selected && t.Status == batchstate.Failed {
				changed = true
				t.Status = batchstate.Pending
				// A publication failure must not discard a usable engine-success
				// checkpoint. Recovery will retry publication without ripping.
				if t.StagingInvalid || (t.Verification == nil && !t.EngineSucceeded) {
					t.Verification = nil
					t.EngineSucceeded = false
					t.StagedFile = ""
					t.AttemptID = ""
					t.StagingInvalid = false
				}
			}
		}
		isoStatus(j)
	}
	if changed && s.AllReviewed() {
		s.Status = batchstate.Paused
	}
	return store.Save(s)
}
