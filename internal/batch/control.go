package batch

import (
	"context"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"github.com/madarlevi/makemkv-tui/internal/proccontrol"
	"github.com/madarlevi/makemkv-tui/internal/publish"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

type Snapshot struct {
	State           batchstate.State
	Logs            []string
	ProcessProgress float64
	Active          bool
}
type Control struct {
	mu                           sync.Mutex
	s                            *batchstate.State
	store                        Saver
	engine                       backend.Engine
	ctx                          context.Context
	cancel                       context.CancelFunc
	force                        func()
	immediate                    atomic.Int32
	active                       bool
	reconciled                   bool
	logs                         []string
	log                          io.Writer
	persistErr                   error
	lastSave, runStart           time.Time
	baseElapsed, processProgress float64
	// afterPublish is a fault-injection seam used only by crash-recovery tests.
	afterPublish func() error
	publisher    publish.Publisher
}

func NewControl(ctx context.Context, s *batchstate.State, store Saver, engine backend.Engine) *Control {
	ctx, cancel := context.WithCancel(ctx)
	ctx, force := proccontrol.WithForce(ctx)
	return &Control{s: s, store: store, engine: engine, ctx: ctx, cancel: cancel, force: force}
}

// NewRecoveredControl consumes a just-reconciled state for immediate resume,
// avoiding a second full-file hash pass while holding the same state lock.
func NewRecoveredControl(ctx context.Context, s *batchstate.State, store Saver, engine backend.Engine) *Control {
	c := NewControl(ctx, s, store, engine)
	c.reconciled = true
	return c
}
func (c *Control) Context() context.Context { return c.ctx }
func (c *Control) Shutdown()                { c.cancel() }
func (c *Control) SetLog(w io.Writer)       { c.mu.Lock(); defer c.mu.Unlock(); c.log = w }
func (c *Control) save() error {
	if c.active && !c.runStart.IsZero() {
		c.s.ElapsedSeconds = c.baseElapsed + time.Since(c.runStart).Seconds()
	}
	e := c.store.Save(c.s)
	if e != nil {
		c.persistErr = e
		c.cancel()
	}
	c.lastSave = time.Now()
	return e
}
func (c *Control) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := *c.s
	s.Errors = append([]batchstate.Failure(nil), c.s.Errors...)
	s.ISOs = append([]batchstate.ISO(nil), c.s.ISOs...)
	for i := range s.ISOs {
		s.ISOs[i].Errors = append([]batchstate.Failure(nil), c.s.ISOs[i].Errors...)
		s.ISOs[i].Titles = append([]batchstate.Title(nil), c.s.ISOs[i].Titles...)
		for k := range s.ISOs[i].Titles {
			t := &s.ISOs[i].Titles[k]
			t.Errors = append([]batchstate.Failure(nil), t.Errors...)
			if t.Verification != nil {
				v := *t.Verification
				t.Verification = &v
			}
		}
	}
	// Scan metadata is immutable once installed. Share it read-only rather than
	// copying every stream attribute on each dashboard refresh.
	if c.active && !c.runStart.IsZero() {
		s.ElapsedSeconds = c.baseElapsed + time.Since(c.runStart).Seconds()
	}
	return Snapshot{s, append([]string(nil), c.logs...), c.processProgress, c.active}
}
func (c *Control) logLine(s string) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if s == "" {
		return
	}
	c.logs = append(c.logs, s)
	if len(c.logs) > 500 {
		c.logs = c.logs[len(c.logs)-500:]
	}
	if c.log != nil {
		if _, e := fmt.Fprintln(c.log, s); e != nil {
			c.logs = append(c.logs, "Log write error: "+e.Error())
			c.log = nil
		}
	}
}
func (c *Control) Emit(e backend.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logLine(e.Line)
	if e.Operation != "" {
		c.s.Operation = e.Operation
	}
	if e.Progress {
		c.processProgress = e.Current
		c.s.Progress.CurrentFraction = e.Total
		c.s.Recount()
	}
	if c.active && time.Since(c.lastSave) >= time.Second && c.persistErr == nil {
		_ = c.save()
	}
}
func (c *Control) StopNow() error {
	if c.immediate.Add(1) > 1 {
		c.force()
		return nil
	}
	c.cancel() // Cancellation is never delayed by disk I/O or the scheduler mutex.
	c.mu.Lock()
	defer c.mu.Unlock()
	c.s.Stop = batchstate.StopRequest{Mode: "immediate", RequestedAt: time.Now().UTC(), ISO: c.s.CurrentISO}
	if c.active && (c.s.Status == batchstate.Running || c.s.Status == batchstate.Stopping) {
		c.s.Status = batchstate.Stopping
	} else if c.s.Status != batchstate.Planning && c.s.Status != batchstate.Completed && c.s.Status != batchstate.Failed {
		c.s.Status = batchstate.Paused
	}
	c.logLine("Immediate stop requested; terminating active subprocess and preserving completed files")
	return c.save()
}
func (c *Control) StopAfterISO() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active {
		return fmt.Errorf("no batch extraction is running")
	}
	if c.immediate.Load() > 0 {
		return nil
	}
	c.s.Stop = batchstate.StopRequest{Mode: "after_iso", RequestedAt: time.Now().UTC(), ISO: c.s.CurrentISO}
	c.s.Status = batchstate.Stopping
	c.logLine("Stop after current ISO requested; remaining selected titles in this ISO will finish")
	return c.save()
}
func (c *Control) Navigate(i int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i < 0 || i >= len(c.s.ISOs) {
		return fmt.Errorf("ISO position out of range")
	}
	c.s.PlanningISO = i
	return c.save()
}
func (c *Control) Metadata(i int, d *media.Disc) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.s.StartedAt.IsZero() {
		return fmt.Errorf("cannot replace metadata after execution starts")
	}
	if e := SetMetadata(c.s, i, d); e != nil {
		return e
	}
	return c.save()
}
func (c *Control) Selection(i int, ids []int, p preferences.Preferences, reviewed bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := SetSelection(c.s, i, ids, p, reviewed); e != nil {
		return e
	}
	return c.save()
}
func (c *Control) ScanFailure(i int, e error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	j := &c.s.ISOs[i]
	j.Status = batchstate.Failed
	addError(c.s, j, nil, e)
	return c.save()
}
func (c *Control) Retry() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active {
		return fmt.Errorf("batch is running")
	}
	c.reconciled = false
	return RetryFailed(c.s, c.store)
}
func (c *Control) Confirm() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.s.AllReviewed() {
		return fmt.Errorf("review every ISO before extraction")
	}
	c.reconciled = false
	c.s.ConfirmedAt = time.Now().UTC()
	return c.save()
}
func (c *Control) Scan(i int) (*media.Disc, error) {
	return c.ScanContext(c.ctx, i)
}
func (c *Control) ScanContext(ctx context.Context, i int) (*media.Disc, error) {
	c.mu.Lock()
	rel := c.s.ISOs[i].RelativePath
	root := c.s.InputRoot
	c.mu.Unlock()
	if engine, ok := c.engine.(backend.BatchScanner); ok {
		return engine.ScanBatch(ctx, source(root, rel), c.Emit)
	}
	return c.engine.Scan(ctx, source(root, rel), c.Emit)
}
