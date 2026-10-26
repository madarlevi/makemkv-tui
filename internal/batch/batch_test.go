package batch

import (
	"context"
	"errors"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPreparedCheckpointRecoveryWithoutNewExtraction(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	title := &s.ISOs[0].Titles[0]
	title.Status = batchstate.Running
	title.AttemptID = batchstate.ID()
	title.Attempts = 1
	s.Status = batchstate.Running
	s.StartedAt = time.Now()
	s.ConfirmedAt = time.Now()
	s.CurrentISO = 0
	s.CurrentTitle = 2
	candidate := batchstate.Candidate(s, 0, title)
	path := filepath.Join(s.OutputRoot, filepath.FromSlash(candidate))
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, movie, 0600)
	root, e := openOutput(s)
	if e != nil {
		t.Fatal(e)
	}
	title.Verification, e = verify(context.Background(), root, candidate, true)
	root.Close()
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Save(s); e != nil {
		t.Fatal(e)
	}
	q := load(t, store)
	if e = Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	if q.ISOs[0].Titles[0].Status != batchstate.Completed || q.Status != batchstate.Completed {
		t.Fatal(q.Status)
	}
	f := &fakeEngine{}
	if e = NewControl(context.Background(), q, store, f).Run(); e != nil || len(f.calls) != 0 {
		t.Fatal(f.calls, e)
	}
}
func TestCompletedFileHashMismatchRetained(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	if e := NewControl(context.Background(), s, store, &fakeEngine{}).Run(); e != nil {
		t.Fatal(e)
	}
	q := load(t, store)
	path := filepath.Join(q.OutputRoot, "disc", "title2.mkv")
	changed := append(append([]byte{}, movie...), byte('X'))
	os.WriteFile(path, changed, 0600)
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	if q.Status != batchstate.Failed || q.ISOs[0].Titles[0].Status != batchstate.Failed {
		t.Fatal(q.Status)
	}
	data, _ := os.ReadFile(path)
	if !reflect.DeepEqual(data, changed) {
		t.Fatal("corrupt existing file was deleted/replaced")
	}
}
func TestGracefulStopFailurePolicy(t *testing.T) {
	s, store := makePlan(t, []string{"a.iso", "b.iso"}, 2, 7)
	var c *Control
	f := &fakeEngine{hook: func(context.Context, backend.Request) error {
		if e := c.StopAfterISO(); e != nil {
			return e
		}
		return errors.New("disc error")
	}}
	c = NewControl(context.Background(), s, store, f)
	if c.Run() == nil {
		t.Fatal("failure not surfaced")
	}
	q := load(t, store)
	if q.Status != batchstate.Failed || q.Stop.Mode != "after_iso" || len(f.calls) != 1 || q.ISOs[0].Titles[0].Status == batchstate.Completed {
		t.Fatal(q.Status, q.Stop)
	}
}

var movie = []byte{0x1a, 0x45, 0xdf, 0xa3, 'm', 'o', 'v', 'i', 'e'}

type fakeEngine struct {
	mu    sync.Mutex
	calls []string
	hook  func(context.Context, backend.Request) error
}

func (f *fakeEngine) Capabilities() backend.Capabilities       { return backend.Capabilities{} }
func (f *fakeEngine) NormalizeSource(s string) (string, error) { return s, nil }
func (f *fakeEngine) Drives(context.Context, func(backend.Event)) ([]backend.Drive, error) {
	return nil, nil
}
func disc(ids ...int) *media.Disc {
	d := media.NewDisc()
	for _, id := range ids {
		d.Titles[id] = &media.Title{ID: id, Attributes: map[int]string{2: fmt.Sprintf("Title %d", id), 9: "1:00:00", 11: "9"}, Streams: map[int]map[int]string{3: {1: "Audio", 3: "eng"}}}
	}
	return d
}
func (f *fakeEngine) Scan(context.Context, string, func(backend.Event)) (*media.Disc, error) {
	return disc(2, 7, 9), nil
}
func (f *fakeEngine) Extract(ctx context.Context, r backend.Request, emit func(backend.Event)) ([]backend.Artifact, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("%s:%d", r.Source, r.IDs[0]))
	f.mu.Unlock()
	if f.hook != nil {
		if e := f.hook(ctx, r); e != nil {
			return nil, e
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if e := os.MkdirAll(r.Output, 0700); e != nil {
		return nil, e
	}
	p := filepath.Join(r.Output, r.Filenames[r.IDs[0]])
	if e := os.WriteFile(p, movie, 0600); e != nil {
		return nil, e
	}
	emit(backend.Event{Progress: true, Current: 1, Total: 1, Line: "mock success"})
	return []backend.Artifact{{TitleID: r.IDs[0], Path: p}}, nil
}
func makePlan(t *testing.T, paths []string, ids ...int) (*batchstate.State, *batchstate.Store) {
	t.Helper()
	root := t.TempDir()
	in, out := filepath.Join(root, "input ISOs"), filepath.Join(root, "output")
	for _, rel := range paths {
		p := filepath.Join(in, filepath.FromSlash(rel))
		if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte("ISO source"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	s, e := New(in, out, preferences.Defaults(), "")
	if e != nil {
		t.Fatal(e)
	}
	for i := range s.ISOs {
		if e = SetMetadata(s, i, disc(ids...)); e != nil {
			t.Fatal(e)
		}
		if e = SetSelection(s, i, ids, preferences.Defaults(), true); e != nil {
			t.Fatal(e)
		}
	}
	store, e := batchstate.Open(filepath.Join(out, "batch-state.json"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	if e = store.Save(s); e != nil {
		t.Fatal(e)
	}
	return s, store
}
func load(t *testing.T, store *batchstate.Store) *batchstate.State {
	t.Helper()
	s, e := store.Load()
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestDiscoveryHierarchyUnicodeAndBasenames(t *testing.T) {
	s, _ := makePlan(t, []string{"alma/körte.ISO", "barack/körte.iso", "nested/季節 & $/disc one.iSo", "narancs.iso"}, 2)
	got := []string{}
	for _, j := range s.ISOs {
		got = append(got, j.RelativePath)
	}
	want := []string{"alma/körte.ISO", "barack/körte.iso", "narancs.iso", "nested/季節 & $/disc one.iSo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if s.ISOs[0].Titles[0].Output != "alma/körte/title2.mkv" || s.ISOs[1].Titles[0].Output != "barack/körte/title2.mkv" {
		t.Fatal(s.ISOs)
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "external.iso"), []byte("external"), 0600)
	if e := os.Symlink(s.InputRoot, filepath.Join(s.InputRoot, "loop")); e == nil {
		_ = os.Symlink(outside, filepath.Join(s.InputRoot, "external"))
		_, jobs, e := Discover(s.InputRoot)
		if e != nil || len(jobs) != 4 {
			t.Fatal(jobs, e)
		}
	}
}
func TestSelectionsPersistAndPlanningCannotExtract(t *testing.T) {
	s, store := makePlan(t, []string{"a.iso", "b.iso"}, 2, 7)
	if e := SetSelection(s, 0, []int{7}, preferences.Defaults(), true); e != nil {
		t.Fatal(e)
	}
	if e := store.Save(s); e != nil {
		t.Fatal(e)
	}
	q := load(t, store)
	if !reflect.DeepEqual(Selected(q.ISOs[0]), []int{7}) || q.ISOs[0].Titles[0].Status != batchstate.Skipped {
		t.Fatal(q.ISOs[0])
	}
	f := &fakeEngine{}
	c := NewControl(context.Background(), q, store, f)
	q.ISOs[1].Reviewed = false
	q.Status = batchstate.Planning
	if c.Run() == nil || len(f.calls) != 0 {
		t.Fatal("extracted during planning")
	}
}
func TestSequentialOutputsAndCompletedResume(t *testing.T) {
	s, store := makePlan(t, []string{"a/movie.iso", "b/movie.ISO"}, 7, 2)
	f := &fakeEngine{}
	c := NewControl(context.Background(), s, store, f)
	if e := c.Run(); e != nil {
		t.Fatal(e)
	}
	q := load(t, store)
	if q.Status != batchstate.Completed || q.Progress.Completed != 4 {
		t.Fatal(q.Status, q.Progress)
	}
	for _, j := range q.ISOs {
		for _, title := range j.Titles {
			if title.Verification == nil || title.Verification.Size != int64(len(movie)) {
				t.Fatal(title)
			}
			if _, e := os.Stat(filepath.Join(q.OutputRoot, filepath.FromSlash(title.Output))); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	f2 := &fakeEngine{}
	if e := NewControl(context.Background(), q, store, f2).Run(); e != nil || len(f2.calls) != 0 {
		t.Fatal(f2.calls, e)
	}
	if !strings.HasSuffix(f.calls[0], ":2") || !strings.HasSuffix(f.calls[1], ":7") {
		t.Fatal(f.calls)
	}
}
func TestImmediateStopAndTitleResume(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2, 7, 9)
	started := make(chan struct{})
	f := &fakeEngine{}
	f.hook = func(ctx context.Context, r backend.Request) error {
		if r.IDs[0] == 7 {
			os.WriteFile(filepath.Join(r.Output, "partial.mkv"), movie, 0600)
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	c := NewControl(context.Background(), s, store, f)
	done := make(chan error, 1)
	go func() { done <- c.Run() }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("did not start")
	}
	if e := c.StopNow(); e != nil {
		t.Fatal(e)
	}
	if e := c.StopNow(); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop hung")
	}
	q := load(t, store)
	if q.Status != batchstate.Paused || q.ISOs[0].Titles[0].Status != batchstate.Completed || q.ISOs[0].Titles[1].Status != batchstate.Interrupted || q.Stop.Mode != "immediate" {
		t.Fatal(q.Status, q.ISOs[0].Titles)
	}
	stage := filepath.Join(q.OutputRoot, filepath.FromSlash(batchstate.StageDir(q, 0, &q.ISOs[0].Titles[1])))
	if _, e := os.Stat(filepath.Join(stage, "partial.mkv")); e != nil {
		t.Fatal("interrupted data must remain isolated in its old attempt", e)
	}
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	f2 := &fakeEngine{}
	if e := NewControl(context.Background(), q, store, f2).Run(); e != nil {
		t.Fatal(e)
	}
	if len(f2.calls) != 2 || !strings.HasSuffix(f2.calls[0], ":7") || q.ISOs[0].Titles[1].Retries != 1 {
		t.Fatal(f2.calls, q.ISOs[0].Titles)
	}
}
func TestGracefulStopFinishesCurrentISO(t *testing.T) {
	s, store := makePlan(t, []string{"a.iso", "b.iso"}, 2, 7)
	f := &fakeEngine{}
	var c *Control
	once := sync.Once{}
	f.hook = func(context.Context, backend.Request) error {
		var e error
		once.Do(func() { e = c.StopAfterISO() })
		return e
	}
	c = NewControl(context.Background(), s, store, f)
	if e := c.Run(); e != nil {
		t.Fatal(e)
	}
	q := load(t, store)
	if len(f.calls) != 2 || q.Status != batchstate.Paused || q.ISOs[0].Status != batchstate.Completed || q.ISOs[1].Titles[0].Status != batchstate.Pending || q.Stop.Mode != "after_iso" {
		t.Fatal(q.Status, f.calls, q.ISOs)
	}
	f2 := &fakeEngine{}
	if e := NewControl(context.Background(), q, store, f2).Run(); e != nil || len(f2.calls) != 2 {
		t.Fatal(f2.calls, e)
	}
}
func TestCrashBetweenPublishAndCommit(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2, 7)
	f := &fakeEngine{}
	c := NewControl(context.Background(), s, store, f)
	crash := errors.New("power loss")
	c.afterPublish = func() error { return crash }
	if e := c.Run(); !errors.Is(e, crash) {
		t.Fatal(e)
	}
	q := load(t, store)
	if q.ISOs[0].Titles[0].Status != batchstate.Running || q.ISOs[0].Titles[0].Verification == nil {
		t.Fatal("checkpoint missing")
	}
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	if q.ISOs[0].Titles[0].Status != batchstate.Completed {
		t.Fatal("final output not reconciled")
	}
	f2 := &fakeEngine{}
	if e := NewControl(context.Background(), q, store, f2).Run(); e != nil || len(f2.calls) != 1 || !strings.HasSuffix(f2.calls[0], ":7") {
		t.Fatal(f2.calls, e)
	}
}
func TestStaleRunningPartialRecovery(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	title := &s.ISOs[0].Titles[0]
	title.Status = batchstate.Running
	title.AttemptID = batchstate.ID()
	title.Attempts = 1
	s.Status = batchstate.Running
	s.StartedAt = time.Now()
	s.ConfirmedAt = time.Now()
	s.CurrentISO = 0
	s.CurrentTitle = 2
	stage := filepath.Join(s.OutputRoot, filepath.FromSlash(batchstate.StageDir(s, 0, title)))
	os.MkdirAll(stage, 0700)
	os.WriteFile(filepath.Join(stage, "partial.mkv"), movie, 0600)
	if e := store.Save(s); e != nil {
		t.Fatal(e)
	}
	q := load(t, store)
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	if q.ISOs[0].Titles[0].Status != batchstate.Interrupted || q.Status != batchstate.Paused {
		t.Fatal(q)
	}
	if _, e := os.Stat(filepath.Join(stage, "partial.mkv")); e != nil {
		t.Fatal("stale attempt data must remain isolated, not deleted", e)
	}
	f := &fakeEngine{}
	if e := NewControl(context.Background(), q, store, f).Run(); e != nil || len(f.calls) != 1 {
		t.Fatal(f.calls, e)
	}
}
func TestCollisionNeverAdoptedOrOverwritten(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	out := filepath.Join(s.OutputRoot, "disc", "title2.mkv")
	os.MkdirAll(filepath.Dir(out), 0755)
	os.WriteFile(out, movie, 0600)
	f := &fakeEngine{}
	if NewControl(context.Background(), s, store, f).Run() == nil || len(f.calls) != 0 {
		t.Fatal("collision accepted")
	}
	data, _ := os.ReadFile(out)
	if !reflect.DeepEqual(data, movie) {
		t.Fatal("overwritten")
	}
	if load(t, store).Status != batchstate.Failed {
		t.Fatal("failure not saved")
	}
}
func TestFailedTitleRetry(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2, 7)
	f := &fakeEngine{hook: func(_ context.Context, r backend.Request) error {
		if r.IDs[0] == 7 {
			return errors.New("read error")
		}
		return nil
	}}
	if NewControl(context.Background(), s, store, f).Run() == nil {
		t.Fatal("failure ignored")
	}
	q := load(t, store)
	if q.Progress.Completed != 1 || q.Progress.Failed != 1 {
		t.Fatal(q.Progress)
	}
	if e := RetryFailed(q, store); e != nil {
		t.Fatal(e)
	}
	f2 := &fakeEngine{}
	if e := NewControl(context.Background(), q, store, f2).Run(); e != nil || len(f2.calls) != 1 || q.ISOs[0].Titles[1].Retries != 1 {
		t.Fatal(f2.calls, e)
	}
	if len(q.ISOs[0].Titles[1].Errors) == 0 {
		t.Fatal("error history lost")
	}
}
func TestDisconnectCancellation(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeEngine{hook: func(ctx context.Context, _ backend.Request) error { cancel(); <-ctx.Done(); return ctx.Err() }}
	if e := NewControl(ctx, s, store, f).Run(); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	q := load(t, store)
	if q.ISOs[0].Titles[0].Status != batchstate.Interrupted || q.Stop.Mode != "immediate" {
		t.Fatal(q)
	}
}
func TestChangedSourceAndOutputSymlinkRejected(t *testing.T) {
	t.Run("changed source", func(t *testing.T) {
		s, store := makePlan(t, []string{"disc.iso"}, 2)
		if e := os.WriteFile(filepath.Join(s.InputRoot, "disc.iso"), []byte("different ISO data"), 0600); e != nil {
			t.Fatal(e)
		}
		if e := Recover(context.Background(), s, store); e == nil {
			t.Fatal("changed source accepted")
		}
	})
	t.Run("output symlink", func(t *testing.T) {
		s, store := makePlan(t, []string{"disc.iso"}, 2)
		target := t.TempDir()
		canary := filepath.Join(target, "untouched.txt")
		if e := os.WriteFile(canary, []byte("retain this file"), 0600); e != nil {
			t.Fatal(e)
		}
		link := filepath.Join(s.OutputRoot, "disc")
		if e := os.Symlink(target, link); e != nil {
			t.Skipf("directory symlinks unavailable: %v", e)
		}
		assertRecoveryFailureSaved(t, s, store, "symlink")
		st, e := os.Lstat(link)
		if e != nil || st.Mode()&os.ModeSymlink == 0 {
			t.Fatal("recovery changed the link", e)
		}
		data, e := os.ReadFile(canary)
		if e != nil || string(data) != "retain this file" {
			t.Fatal("recovery changed the linked directory", e)
		}
		entries, e := os.ReadDir(target)
		if e != nil || len(entries) != 1 {
			t.Fatal("recovery wrote into the linked directory", entries, e)
		}
	})
}

// Per-title recovery failures are deliberately saved and displayed in the TUI;
// a nil Recover error means that reconciliation was persisted, not that every
// title succeeded. A failed title must still block the execution queue.
func assertRecoveryFailureSaved(t *testing.T, s *batchstate.State, store *batchstate.Store, message string) {
	t.Helper()
	if e := Recover(context.Background(), s, store); e != nil {
		t.Fatal("could not persist recovery failure", e)
	}
	q := load(t, store)
	j := q.ISOs[0]
	title := j.Titles[0]
	if q.Status != batchstate.Failed || j.Status != batchstate.Failed || title.Status != batchstate.Failed || q.Progress.Failed != 1 {
		t.Fatal("unsafe output was not persisted as failed", q.Status, j.Status, title.Status, q.Progress)
	}
	for _, failures := range [][]batchstate.Failure{q.Errors, j.Errors, title.Errors} {
		if len(failures) == 0 || !strings.Contains(failures[0].Message, message) {
			t.Fatal("missing recovery diagnostic", failures)
		}
	}
	f := &fakeEngine{}
	if e := NewControl(context.Background(), q, store, f).Run(); e == nil || len(f.calls) != 0 {
		t.Fatal("failed recovery started extraction", e, f.calls)
	}
}

func TestRecoveryFailureIsPersistedBeforeExtraction(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	// This exercises the same recovery failure policy without symlink privileges.
	// A directory at the final filename is an unverified collision, never an MKV.
	out := filepath.Join(s.OutputRoot, "disc", "title2.mkv")
	if e := os.MkdirAll(out, 0700); e != nil {
		t.Fatal(e)
	}
	assertRecoveryFailureSaved(t, s, store, "unverified output collision")
	if st, e := os.Stat(out); e != nil || !st.IsDir() {
		t.Fatal("recovery changed the existing output", e)
	}
}
