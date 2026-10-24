package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batch"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/makemkv"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"github.com/madarlevi/makemkv-tui/internal/tui"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type batchFlags struct {
	input, output, resume, state, config, profile string
	retry                                         bool
	newPlan, plans, list, stateDir                bool
}

// flag.FlagSet stops at a positional argument; reorder known options so the
// documented "batch INPUT --output OUTPUT" syntax works without shell parsing.
func parseBatch(args []string, w io.Writer) (batchFlags, error) {
	var b batchFlags
	defaultConfig, e := preferences.Path()
	if e != nil {
		return b, e
	}
	f := flag.NewFlagSet("batch", flag.ContinueOnError)
	f.SetOutput(w)
	f.StringVar(&b.output, "output", "", "output root (new batch)")
	f.StringVar(&b.resume, "resume", "", "resume an existing state file")
	f.StringVar(&b.state, "state", "", "explicit state JSON path (including legacy locations)")
	f.BoolVar(&b.newPlan, "new", false, "explicitly create a separate plan even if an unfinished matching plan exists")
	f.BoolVar(&b.plans, "plans", false, "choose a saved batch to resume in the TUI")
	f.BoolVar(&b.list, "list", false, "list saved batch paths and statuses")
	f.BoolVar(&b.stateDir, "state-dir", false, "print the default local batch-state directory")
	f.StringVar(&b.config, "config", defaultConfig, "frontend preferences JSON")
	f.StringVar(&b.profile, "profile", "", "existing MakeMKV profile (new batch)")
	f.BoolVar(&b.retry, "retry-failed", false, "explicitly retry failed titles when resuming")
	f.Usage = func() {
		fmt.Fprintln(w, "usage: makemkv-tui batch INPUT --output OUTPUT [--state PATH] [--new]\n       makemkv-tui batch --resume STATE [--retry-failed]\n       makemkv-tui batch --plans | --list | --state-dir\nDefault plans: $XDG_STATE_HOME/makemkv-tui/batches/ or $HOME/.local/state/makemkv-tui/batches/\nResume legacy plans with their explicit path; their location is preserved.")
		f.PrintDefaults()
	}
	flags, pos := []string{}, []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
		option := f.Lookup(name)
		if name == "h" || name == "help" {
			f.Usage()
			return b, flag.ErrHelp
		}
		if option == nil {
			return b, fmt.Errorf("unknown batch option %q", a)
		}
		flags = append(flags, a)
		boolFlag, isBool := option.Value.(interface{ IsBoolFlag() bool })
		if !(isBool && boolFlag.IsBoolFlag()) && !strings.Contains(a, "=") {
			i++
			if i >= len(args) {
				return b, fmt.Errorf("missing value for %s", a)
			}
			flags = append(flags, args[i])
		}
	}
	if e = f.Parse(flags); e != nil {
		return b, e
	}
	if len(pos) > 1 {
		return b, fmt.Errorf("batch accepts one input directory; quote paths containing spaces")
	}
	if len(pos) == 1 {
		b.input = pos[0]
	}
	commands := 0
	for _, yes := range []bool{b.plans, b.list, b.stateDir} {
		if yes {
			commands++
		}
	}
	if commands > 0 {
		if commands != 1 || b.input != "" || b.output != "" || b.resume != "" || b.state != "" || b.profile != "" || b.newPlan || (b.retry && !b.plans) {
			return b, fmt.Errorf("use --plans, --list or --state-dir separately from new-plan paths")
		}
		return b, nil
	}
	if b.resume != "" {
		if b.input != "" || b.output != "" || b.state != "" || b.profile != "" || b.newPlan {
			return b, fmt.Errorf("--resume uses the saved input/output/profile; do not combine it with new-plan paths")
		}
	} else {
		if b.input == "" || b.output == "" {
			return b, fmt.Errorf("batch requires INPUT and --output OUTPUT, or --resume STATE")
		}
		if b.retry {
			return b, fmt.Errorf("--retry-failed requires --resume")
		}
	}
	return b, nil
}
func runBatch(args []string) error {
	b, e := parseBatch(args, os.Stderr)
	if errors.Is(e, flag.ErrHelp) {
		return nil
	}
	if e != nil {
		return e
	}
	if b.plans || b.list || b.stateDir {
		dir, err := batchstate.Directory()
		if err != nil {
			return err
		}
		if b.stateDir {
			fmt.Fprintln(os.Stdout, dir)
			return nil
		}
		plans, err := batchstate.Plans(dir)
		if err != nil {
			return err
		}
		if b.list {
			fmt.Fprintln(os.Stdout, "Batch plans:", dir)
			for _, p := range plans {
				if p.Err != nil {
					fmt.Fprintf(os.Stdout, "%s · ERROR: %s\n", p.Path, p.Err)
				} else {
					fmt.Fprintf(os.Stdout, "%s · %s · %d/%d titles\n", p.Path, p.State.Status, p.State.Progress.Completed, p.State.Progress.Selected)
				}
			}
			return nil
		}
		chosen, err := tea.NewProgram(tui.NewSavedPlans(plans), tea.WithAltScreen()).Run()
		if err != nil {
			return err
		}
		b.resume = chosen.(tui.SavedPlans).Selected
		if b.resume == "" {
			return nil
		}
	}
	path := b.state
	var state *batchstate.State
	var registry *batchstate.Store
	if b.resume != "" {
		path = b.resume
	} else {
		prefs, err := preferences.Load(b.config)
		if err != nil {
			return err
		}
		state, e = batch.New(b.input, b.output, prefs, b.profile)
		if e != nil {
			return e
		}
		path, e = batchstate.PlanPath(b.state, state)
		if e != nil {
			return e
		}
		// Serialize duplicate detection and publication across new batch IDs.
		registry, e = batchstate.Open(filepath.Join(filepath.Dir(path), ".catalog"))
		if e != nil {
			return e
		}
		defer registry.Close()
		if !b.newPlan {
			plans, err := batchstate.Plans(filepath.Dir(path))
			if err != nil {
				return err
			}
			match, err := batchstate.Matching(plans, state)
			if err != nil {
				return err
			}
			if match != "" {
				return fmt.Errorf("unfinished matching plan: %s\nResume: makemkv-tui batch --resume %q\nOr choose it with batch --plans. To explicitly create a separate plan, repeat the command with --new", match, match)
			}
		}
	}
	store, e := batchstate.Open(path)
	if e != nil {
		return e
	}
	defer store.Close()
	fmt.Fprintln(os.Stderr, "Batch state:", store.Path)
	if store.StaleLock {
		fmt.Fprintln(os.Stderr, "Previous lock metadata found; the OS lock is free and has been reacquired safely.")
	}
	if b.resume != "" {
		state, e = store.Load()
		if e != nil {
			return e
		}
		recoverCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		fmt.Fprintln(os.Stderr, "Verifying saved outputs and recovering title-level state…")
		e = batch.Recover(recoverCtx, state, store)
		stop()
		if e != nil {
			return e
		}
		if b.retry {
			if e = batch.RetryFailed(state, store); e != nil {
				return e
			}
		}
	} else {
		if store.Exists() {
			return fmt.Errorf("state already exists at %s; use batch --resume %s or choose --state elsewhere", store.Path, store.Path)
		}
		if e = store.Initialize(state); e != nil {
			return e
		}
		if e = registry.Close(); e != nil {
			return e
		}
	}
	x, e := makemkv.Detect()
	if e != nil {
		return e
	} // The plan remains saved even when the engine is unavailable.
	log, e := store.OpenLog()
	if e != nil {
		return e
	}
	defer log.Close()
	autoResume := b.resume != "" && !state.ConfirmedAt.IsZero() && state.AllReviewed() && state.Progress.Failed == 0 && state.Status != batchstate.Completed
	c := batch.NewControl(context.Background(), state, store, backend.MakeMKV{Runner: x})
	if autoResume {
		c.Shutdown()
		c = batch.NewRecoveredControl(context.Background(), state, store, backend.MakeMKV{Runner: x})
	}
	c.SetLog(log)
	opts := tui.Options{Preferences: state.Preferences, ConfigPath: b.config, Profile: state.Profile, Log: log, LogPath: log.Name()}
	p := tea.NewProgram(tui.NewBatch(c, backend.MakeMKV{Runner: x}, opts, autoResume), tea.WithAltScreen(), tea.WithoutSignalHandler())
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	pumpDone := make(chan struct{})
	var wg sync.WaitGroup
	go func() {
		defer close(pumpDone)
		for {
			select {
			case <-signals:
				wg.Add(1)
				go func() { defer wg.Done(); e := c.StopNow(); p.Send(tui.BatchStopMsg{AlreadyRequested: true, Err: e}) }()
			case <-done:
				return
			}
		}
	}()
	_, e = p.Run()
	c.Shutdown() // Cancel before taking snapshots: recovery may currently hold the state mutex while hashing.
	signal.Stop(signals)
	close(done)
	<-pumpDone
	// Stop a worker if the terminal renderer fails or the SSH/PTY disappears.
	snap := c.Snapshot()
	if snap.Active {
		_ = c.StopNow()
	}
	c.Shutdown()
	// The TUI waits for normal stop completion; renderer failure must also wait
	// until the worker has persisted its last transition before releasing the lock.
	for c.Snapshot().Active {
		timeWait()
	}
	wg.Wait()
	snap = c.Snapshot()
	fmt.Fprintf(os.Stderr, "Batch %s · completed %d/%d titles · state: %s\n", snap.State.Status, snap.State.Progress.Completed, snap.State.Progress.Selected, store.Path)
	if e != nil {
		return e
	}
	if snap.State.Status == batchstate.Failed {
		return fmt.Errorf("batch failed; inspect %s.log and resume with --retry-failed after resolving the error", store.Path)
	}
	return nil
}

func timeWait() { time.Sleep(20 * time.Millisecond) }
