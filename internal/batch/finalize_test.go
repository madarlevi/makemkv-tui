package batch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/publish"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type completedRunner struct{ mkv int }

func (f *completedRunner) Run(ctx context.Context, args []string, emit func(string)) error {
	for _, arg := range args {
		if arg == "info" {
			emit(`TCOUNT:1`)
			emit(`TINFO:2,2,0,"Title 2"`)
			emit(`TINFO:2,9,0,"1:00:00"`)
			emit(`TINFO:2,11,0,"9"`)
			return nil
		}
	}
	f.mkv++
	if e := os.WriteFile(filepath.Join(args[len(args)-1], "title_t00.mkv"), movie, 0600); e != nil {
		return e
	}
	emit(`MSG:5005,0,1,"Copy complete. 1 titles saved.","Copy complete. %1 titles saved.","1"`)
	return nil
}
func noRename(*os.File, string, *os.File, string) error { return publish.ErrNoReplace }
func stagedBytes(t *testing.T, s *batchstate.State) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(s.OutputRoot, filepath.FromSlash(batchstate.Candidate(s, 0, &s.ISOs[0].Titles[0]))))
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestPublishEIOPreservesRealEngineOutputAndRetryDoesNotRerip(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	f := &completedRunner{}
	c := NewControl(context.Background(), s, store, backend.MakeMKV{Runner: f})
	c.publisher.Rename = func(*os.File, string, *os.File, string) error {
		return &os.LinkError{Op: "renameat2", Old: "title_t00.mkv", New: "title2.mkv", Err: syscall.EIO}
	}
	e := c.Run()
	if !errors.Is(e, syscall.EIO) || f.mkv != 1 {
		t.Fatal(e, f.mkv)
	}
	q := load(t, store)
	title := &q.ISOs[0].Titles[0]
	if title.Status != batchstate.Failed || title.Verification == nil || title.StagedFile != "title_t00.mkv" || !bytes.Equal(stagedBytes(t, q), movie) {
		t.Fatal("completed extraction lost")
	}
	if len(title.Errors) == 0 || !strings.Contains(title.Errors[0].Message, syscall.EIO.Error()) || !strings.Contains(title.Errors[0].Message, q.OutputRoot) {
		t.Fatal("filesystem error truncated", title.Errors)
	}
	attempt := title.AttemptID
	if e = RetryFailed(q, store); e != nil {
		t.Fatal(e)
	}
	if title.Verification == nil || title.AttemptID != attempt {
		t.Fatal("retry discarded verified staging")
	}
	if e = NewControl(context.Background(), q, store, backend.MakeMKV{Runner: f}).Run(); e != nil || f.mkv != 1 {
		t.Fatal("reripped instead of publishing", e, f.mkv)
	}
	final, e := os.ReadFile(filepath.Join(q.OutputRoot, filepath.FromSlash(title.Output)))
	if e != nil || !bytes.Equal(final, movie) || title.Status != batchstate.Completed {
		t.Fatal(e)
	}
}

type successCheckpointFault struct{ Saver }

func (f successCheckpointFault) Save(s *batchstate.State) error {
	for _, iso := range s.ISOs {
		for _, title := range iso.Titles {
			if title.EngineSucceeded && title.Verification == nil {
				return errors.New("crash before success checkpoint")
			}
		}
	}
	return f.Saver.Save(s)
}
func TestReceiptRecoversCrashBeforeJSONAndHashWithoutRipping(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	f := &completedRunner{}
	if e := NewControl(context.Background(), s, successCheckpointFault{store}, backend.MakeMKV{Runner: f}).Run(); e == nil {
		t.Fatal("crash not simulated")
	}
	q := load(t, store)
	if q.ISOs[0].Titles[0].EngineSucceeded || q.ISOs[0].Titles[0].Verification != nil {
		t.Fatal("checkpoint should not be persisted")
	}
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	if q.ISOs[0].Titles[0].Status != batchstate.Completed || f.mkv != 1 {
		t.Fatal("receipt not recovered")
	}
	if e := NewControl(context.Background(), q, store, backend.MakeMKV{Runner: f}).Run(); e != nil || f.mkv != 1 {
		t.Fatal(e, f.mkv)
	}
}

type legacyLinkFailure struct{ fakeEngine }

func (f *legacyLinkFailure) Extract(ctx context.Context, r backend.Request, emit func(backend.Event)) ([]backend.Artifact, error) {
	inner := filepath.Join(r.Output, ".makemkv-tui-45460389")
	if e := os.Mkdir(inner, 0700); e != nil {
		return nil, e
	}
	path := filepath.Join(inner, "title_t00.mkv")
	if e := os.WriteFile(path, movie, 0600); e != nil {
		return nil, e
	}
	emit(backend.Event{Line: "INFO: Copy complete. 1 titles saved."})
	return nil, fmt.Errorf("staged file retained: %w", &os.LinkError{Op: "link", Old: path, New: filepath.Join(r.Output, "title2.mkv"), Err: syscall.EIO})
}
func TestLegacyNestedLinkFailureNeverTriggersRecursivePartialDeletion(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	e := NewControl(context.Background(), s, store, &legacyLinkFailure{}).Run()
	if !errors.Is(e, syscall.EIO) {
		t.Fatal(e)
	}
	q := load(t, store)
	path := filepath.Join(q.OutputRoot, filepath.FromSlash(batchstate.StageDir(q, 0, &q.ISOs[0].Titles[0])), ".makemkv-tui-45460389", "title_t00.mkv")
	b, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(b, movie) {
		t.Fatal("regression: outer cleanup deleted the successful inner MKV", e)
	}
}

func TestPartialFinalCopyCollisionRequiresResolutionButNeverRerips(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	f := &completedRunner{}
	c := NewControl(context.Background(), s, store, backend.MakeMKV{Runner: f})
	c.publisher.Rename = noRename
	copies := 0
	c.publisher.Copy = func(_ context.Context, w io.Writer, r io.Reader) error {
		copies++
		if copies == 2 {
			_, _ = w.Write([]byte{0x1a, 0x45})
			return syscall.EIO
		}
		_, e := io.Copy(w, r)
		return e
	}
	if e := c.Run(); !errors.Is(e, syscall.EIO) {
		t.Fatal(e)
	}
	q := load(t, store)
	title := &q.ISOs[0].Titles[0]
	if !bytes.Equal(stagedBytes(t, q), movie) {
		t.Fatal("source not retained")
	}
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	if title.Status != batchstate.Failed || title.Verification == nil || f.mkv != 1 {
		t.Fatal("partial final incorrectly completed")
	}
	path := filepath.Join(q.OutputRoot, filepath.FromSlash(title.Output))
	b, e := os.ReadFile(path)
	if e != nil || len(b) != 2 {
		t.Fatal("partial final silently replaced", e)
	}
	// Simulate the user's explicit collision resolution. The application never
	// removes or overwrites this path by itself, even if it created the partial.
	if e = os.Rename(path, path+".user-retained-partial"); e != nil {
		t.Fatal(e)
	}
	if e = Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	if title.Status != batchstate.Completed || f.mkv != 1 {
		t.Fatal("did not retry publication")
	}
}

func TestStagingSurvivesFailedCompletionSaveAndFailedRecoverySave(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	c := NewControl(context.Background(), s, failSave{store, true}, &fakeEngine{})
	c.publisher.Rename = noRename // Keep the source file for an explicit cleanup-order assertion.
	if e := c.Run(); e == nil {
		t.Fatal("save failure ignored")
	}
	q := load(t, store)
	if !bytes.Equal(stagedBytes(t, q), movie) {
		t.Fatal("cleaned before completion save")
	}
	if e := Recover(context.Background(), q, unavailableState{store}); e == nil {
		t.Fatal("recovery save failure ignored")
	}
	if !bytes.Equal(stagedBytes(t, q), movie) {
		t.Fatal("recovery cleaned before save")
	}
	q = load(t, store)
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	stage := filepath.Join(q.OutputRoot, filepath.FromSlash(batchstate.StageDir(q, 0, &q.ISOs[0].Titles[0])))
	if _, e := os.Stat(stage); !os.IsNotExist(e) {
		t.Fatal("successful, committed publication did not clean its own attempt", e)
	}
}

type saveHook struct {
	Saver
	hook func(*batchstate.State)
}

func (f saveHook) Save(s *batchstate.State) error { f.hook(s); return f.Saver.Save(s) }
func TestStopAfterEngineSuccessPreservesStagingForPublicationOnResume(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	var c *Control
	saver := saveHook{store, func(s *batchstate.State) {
		if title := s.ISOs[0].Titles[0]; title.EngineSucceeded && title.Verification == nil {
			c.Shutdown()
		}
	}}
	c = NewControl(context.Background(), s, saver, &fakeEngine{})
	if e := c.Run(); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	q := load(t, store)
	if q.ISOs[0].Titles[0].Status != batchstate.Interrupted || !bytes.Equal(stagedBytes(t, q), movie) {
		t.Fatal("stop lost completed extraction")
	}
	if e := Recover(context.Background(), q, store); e != nil {
		t.Fatal(e)
	}
	f := &fakeEngine{}
	if e := NewControl(context.Background(), q, store, f).Run(); e != nil || len(f.calls) != 0 {
		t.Fatal("interrupted finalization reripped", e)
	}
}

func TestBothStopModesRemainResponsiveDuringFinalCopy(t *testing.T) {
	for _, mode := range []string{"immediate", "after_iso"} {
		t.Run(mode, func(t *testing.T) {
			s, store := makePlan(t, []string{"a.iso", "b.iso"}, 2, 7)
			f := &fakeEngine{}
			c := NewControl(context.Background(), s, store, f)
			t.Cleanup(c.Shutdown)
			c.publisher.Rename = noRename
			started, allow := make(chan struct{}), make(chan struct{})
			copies := 0
			c.publisher.Copy = func(ctx context.Context, w io.Writer, r io.Reader) error {
				copies++
				if copies == 2 {
					close(started)
					select {
					case <-allow:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				_, e := io.Copy(w, r)
				return e
			}
			done := make(chan error, 1)
			go func() { done <- c.Run() }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("final copy did not start")
			}
			stopDone := make(chan error, 1)
			go func() {
				if mode == "immediate" {
					stopDone <- c.StopNow()
				} else {
					stopDone <- c.StopAfterISO()
				}
			}()
			select {
			case e := <-stopDone:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(3 * time.Second):
				c.Shutdown()
				t.Fatal("stop was blocked by finalization mutex")
			}
			close(allow)
			select {
			case e := <-done:
				if mode == "immediate" && !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
				if mode == "after_iso" && e != nil {
					t.Fatal(e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("stop did not finish")
			}
			q := load(t, store)
			if q.Status != batchstate.Paused || q.Stop.Mode != mode {
				t.Fatal(q.Status, q.Stop)
			}
			if mode == "after_iso" && (len(f.calls) != 2 || q.ISOs[0].Status != batchstate.Completed || q.ISOs[1].Status != batchstate.Pending) {
				t.Fatal("graceful stop did not finish just the current ISO", f.calls)
			}
			if mode == "immediate" && (len(f.calls) != 1 || q.ISOs[0].Titles[0].Status != batchstate.Interrupted || !bytes.Equal(stagedBytes(t, q), movie)) {
				t.Fatal("immediate stop lost completed source")
			}
		})
	}
}
