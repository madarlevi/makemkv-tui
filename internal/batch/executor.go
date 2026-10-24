package batch

import (
	"context"
	"errors"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func source(root, rel string) string { return "iso:" + filepath.Join(root, filepath.FromSlash(rel)) }
func (c *Control) Run() error {
	c.mu.Lock()
	if c.active {
		c.mu.Unlock()
		return fmt.Errorf("batch is already running")
	}
	if !c.s.AllReviewed() {
		c.mu.Unlock()
		return fmt.Errorf("planning is incomplete; extraction is forbidden")
	}
	if e := c.ctx.Err(); e != nil {
		c.mu.Unlock()
		return e
	}
	var recoveryErr error
	if c.reconciled {
		recoveryErr = ValidateSources(c.s)
	} else {
		recoveryErr = Recover(c.ctx, c.s, c.store)
	}
	c.reconciled = false
	if e := recoveryErr; e != nil {
		c.mu.Unlock()
		return e
	}
	if c.s.Progress.Failed > 0 {
		c.mu.Unlock()
		return fmt.Errorf("failed titles require an explicit retry; use --retry-failed or R in the summary")
	}
	c.active = true
	c.runStart = time.Now()
	c.baseElapsed = c.s.ElapsedSeconds
	if c.s.StartedAt.IsZero() {
		c.s.StartedAt = time.Now().UTC()
	}
	if c.s.ConfirmedAt.IsZero() {
		c.s.ConfirmedAt = time.Now().UTC()
	}
	// A stop that already reached its ISO boundary is consumed by an explicit resume.
	keepStop := false
	if c.s.Stop.Mode == "after_iso" && c.s.Stop.ISO >= 0 && c.s.Stop.ISO < len(c.s.ISOs) {
		for _, t := range c.s.ISOs[c.s.Stop.ISO].Titles {
			if t.Selected && t.Status != batchstate.Completed {
				keepStop = true
			}
		}
	}
	if !keepStop {
		c.s.Stop = batchstate.StopRequest{}
	}
	c.s.Status = batchstate.Running
	if keepStop {
		c.s.Status = batchstate.Stopping
	}
	e := c.save()
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.active = false; c.mu.Unlock() }()
	if e != nil {
		return e
	}
	r, e := openOutput(c.s)
	if e != nil {
		return c.batchFailure(e)
	}
	defer r.Close()
	for i := range c.s.ISOs {
		c.mu.Lock()
		if c.ctx.Err() != nil {
			e = c.pauseLocked()
			c.mu.Unlock()
			return e
		}
		if c.s.Stop.Mode == "after_iso" && i > c.s.Stop.ISO {
			e = c.pauseLocked()
			c.mu.Unlock()
			return e
		}
		j := &c.s.ISOs[i]
		needed := false
		for _, t := range j.Titles {
			if t.Selected && t.Status != batchstate.Completed {
				needed = true
			}
		}
		if !needed {
			c.mu.Unlock()
			continue
		}
		c.s.CurrentISO = i
		c.s.CurrentTitle = -1
		j.Status = batchstate.Running
		j.Attempts++
		j.Retries = max(0, j.Attempts-1)
		if j.StartedAt.IsZero() {
			j.StartedAt = time.Now().UTC()
		}
		e = c.save()
		c.mu.Unlock()
		if e != nil {
			return e
		}
		if e = c.preflightISO(i); e != nil {
			return e
		}
		for k := range c.s.ISOs[i].Titles {
			c.mu.Lock()
			t := &c.s.ISOs[i].Titles[k]
			skip := !t.Selected || t.Status == batchstate.Completed
			c.mu.Unlock()
			if skip {
				continue
			}
			if e = c.title(r, i, k); e != nil {
				return e
			}
		}
		c.mu.Lock()
		isoStatus(&c.s.ISOs[i])
		c.s.ISOs[i].UpdatedAt = time.Now().UTC()
		c.s.CurrentTitle = -1
		c.s.Progress.CurrentFraction = 0
		e = c.save()
		c.mu.Unlock()
		if e != nil {
			return e
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.s.Status = batchstate.Completed
	c.s.Operation = "Batch completed"
	c.s.CompletedAt = time.Now().UTC()
	c.s.CurrentTitle = -1
	c.s.Progress.CurrentFraction = 0
	return c.save()
}
func (c *Control) batchFailure(e error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.s.Status = batchstate.Failed
	addError(c.s, nil, nil, e)
	if se := c.save(); se != nil {
		return se
	}
	return e
}
func (c *Control) pauseLocked() error {
	c.s.Status = batchstate.Paused
	c.s.Operation = "Paused safely"
	all := true
	for _, j := range c.s.ISOs {
		for _, t := range j.Titles {
			if t.Selected && t.Status != batchstate.Completed {
				all = false
			}
		}
	}
	if all {
		c.s.Status = batchstate.Completed
		c.s.CompletedAt = time.Now().UTC()
	}
	c.s.CurrentTitle = -1
	c.s.Progress.CurrentFraction = 0
	if c.s.Stop.Mode == "" {
		c.s.Stop = batchstate.StopRequest{Mode: "immediate", RequestedAt: time.Now().UTC(), ISO: c.s.CurrentISO}
	}
	if c.persistErr != nil {
		return fmt.Errorf("state persistence failed; stopped scheduling: %w", c.persistErr)
	}
	return c.save()
}
func (c *Control) title(r *os.Root, i, k int) error {
	c.mu.Lock()
	s := c.s
	j := &s.ISOs[i]
	t := &j.Titles[k]
	if c.ctx.Err() != nil {
		e := c.pauseLocked()
		c.mu.Unlock()
		return stopped(e)
	}
	t.Status = batchstate.Running
	t.AttemptID = batchstate.ID()
	t.Verification = nil
	t.EngineSucceeded = false
	t.StagedFile = ""
	t.StagingInvalid = false
	t.Attempts++
	t.Retries = max(0, t.Attempts-1)
	t.StartedAt = time.Now().UTC()
	t.UpdatedAt = t.StartedAt
	s.CurrentTitle = t.ID
	s.Progress.CurrentFraction = 0
	c.processProgress = 0
	s.Operation = "Starting extraction"
	stage := batchstate.StageDir(s, i, t)
	id := t.ID
	output := t.Output
	e := c.save()
	c.mu.Unlock()
	if e != nil {
		return e
	}
	if e = checkPath(r, output, true); e != nil {
		return c.titleFailure(r, i, k, e, false)
	}
	if _, e = r.Lstat(filepath.FromSlash(output)); e == nil {
		return c.titleFailure(r, i, k, fmt.Errorf("output collision: %s; never overwritten", output), false)
	} else if !os.IsNotExist(e) {
		return c.titleFailure(r, i, k, e, false)
	}
	if e = checkPath(r, stage, true); e != nil {
		return c.titleFailure(r, i, k, e, false)
	}
	if e = r.MkdirAll(filepath.Dir(filepath.FromSlash(stage)), 0700); e != nil {
		return c.titleFailure(r, i, k, e, false)
	}
	if e = r.Mkdir(filepath.FromSlash(stage), 0700); e != nil {
		return c.titleFailure(r, i, k, e, false)
	}
	if e = syncOutputDir(r, filepath.ToSlash(filepath.Join(filepath.FromSlash(stage), ".entry"))); e != nil {
		return c.titleFailure(r, i, k, e, false)
	}
	// Recheck planned source fingerprints before every title, not only at startup.
	if e = ValidateSources(s); e != nil {
		return c.titleFailure(r, i, k, e, true)
	}
	minimumLength := 0
	req := backend.Request{MinimumLength: &minimumLength, RequireSuccess: true, StagedOnly: true, Source: source(s.InputRoot, j.RelativePath), Output: filepath.Join(s.OutputRoot, filepath.FromSlash(stage)), IDs: []int{id}, Filenames: map[int]string{id: fmt.Sprintf("title%d.mkv", id)}, Profile: s.Profile}
	artifacts, e := c.engine.Extract(c.ctx, req, c.Emit)
	if e != nil {
		return c.titleFailure(r, i, k, e, true)
	}
	expectedDir := filepath.Join(s.OutputRoot, filepath.FromSlash(stage))
	if len(artifacts) != 1 || artifacts[0].TitleID != id || filepath.Dir(filepath.Clean(artifacts[0].Path)) != expectedDir || !strings.EqualFold(filepath.Ext(artifacts[0].Path), ".mkv") {
		return c.titleFailure(r, i, k, fmt.Errorf("engine did not return the expected staged title %d", id), true)
	}
	c.mu.Lock()
	t.StagedFile = filepath.Base(artifacts[0].Path)
	t.EngineSucceeded = true
	s.Operation = "Validating output and computing SHA-256"
	e = c.save() // Save engine success before a long hash, cancellation or disk failure.
	candidate := batchstate.Candidate(s, i, t)
	c.mu.Unlock()
	if e != nil {
		return e
	}
	v, e := verify(c.ctx, r, candidate, true)
	if e != nil {
		return c.titleFailure(r, i, k, e, true)
	}
	if e = syncOutputDir(r, candidate); e != nil {
		return c.titleFailure(r, i, k, e, false)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx.Err() != nil {
		t.Status = batchstate.Interrupted
		j.Status = batchstate.Interrupted
		e = c.pauseLocked()
		return stopped(e)
	}
	t.Verification = v
	s.Operation = "Finalizing verified output"
	t.UpdatedAt = time.Now().UTC()
	// Durable write-ahead checkpoint: success + hash must reach JSON before final publication.
	if e = c.save(); e != nil {
		return e
	}
	if e = c.ctx.Err(); e != nil {
		t.Status = batchstate.Interrupted
		j.Status = batchstate.Interrupted
		se := c.pauseLocked()
		return stopped(se)
	}
	// Copy fallback can take a long time: leave the dashboard and both stop
	// controls responsive while only immutable publication inputs are read.
	c.mu.Unlock()
	e = publishOutput(c.ctx, c.publisher, s, r, i, t)
	c.mu.Lock()
	if e != nil {
		t.Status, j.Status, s.Status = batchstate.Failed, batchstate.Failed, batchstate.Failed
		if c.ctx.Err() != nil {
			t.Status, j.Status, s.Status = batchstate.Interrupted, batchstate.Interrupted, batchstate.Paused
		}
		addError(s, j, t, e)
		c.logLine("ERROR: " + e.Error())
		if se := c.save(); se != nil {
			return errors.Join(e, se)
		}
		return e
	}
	if c.afterPublish != nil {
		if e = c.afterPublish(); e != nil {
			return e
		}
	} // simulate power loss between publication and JSON
	t.Status = batchstate.Completed
	t.CompletedAt = time.Now().UTC()
	t.UpdatedAt = t.CompletedAt
	s.Progress.CurrentFraction = 0
	c.processProgress = 1
	if e = c.save(); e != nil {
		return e
	}
	if e = cleanup(s, r, i, t); e != nil {
		warning := fmt.Errorf("staging cleanup warning (completed output retained): %w", e)
		addError(s, j, t, warning)
		c.logLine("WARNING: " + warning.Error())
		if se := c.save(); se != nil {
			return errors.Join(warning, se)
		}
	}
	c.logLine(fmt.Sprintf("Completed %s (%d bytes)", output, v.Size))
	return nil
}
func (c *Control) titleFailure(r *os.Root, i, k int, e error, _ bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	j := &c.s.ISOs[i]
	t := &j.Titles[k]
	e = fmt.Errorf("%w; all staging data retained in %s", e, filepath.Join(c.s.OutputRoot, filepath.FromSlash(batchstate.StageDir(c.s, i, t))))
	if c.ctx.Err() != nil {
		t.Status = batchstate.Interrupted
		j.Status = batchstate.Interrupted
		c.s.Status = batchstate.Paused
	} else {
		t.Status = batchstate.Failed
		j.Status = batchstate.Failed
		c.s.Status = batchstate.Failed
	}
	t.UpdatedAt = time.Now().UTC()
	c.s.Progress.CurrentFraction = 0
	addError(c.s, j, t, e)
	c.logLine("ERROR: " + e.Error())
	if c.s.Stop.Mode == "" && c.ctx.Err() != nil {
		c.s.Stop = batchstate.StopRequest{Mode: "immediate", RequestedAt: time.Now().UTC(), ISO: i}
	}
	se := c.save()
	if se != nil {
		return errors.Join(e, se)
	}
	if c.ctx.Err() != nil {
		return context.Canceled
	}
	return e
}
func stopped(e error) error {
	if e != nil {
		return e
	}
	return context.Canceled
}
