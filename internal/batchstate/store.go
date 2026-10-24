package batchstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store must remain locked from loading/creating a plan until all workers exit.
type Store struct {
	Path          string
	dir           *os.Root
	lock          *os.File
	mu            sync.Mutex
	StaleLock     bool
	beforeReplace func() error
	createTemp    func(string) (stateFile, error)
	rename        func(string, string) error
	syncParent    func(string) error
}

// Small I/O boundary permits testing failures without a real failing filesystem.
type stateFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

func Open(path string) (*Store, error) {
	abs, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(abs), 0700); e != nil {
		return nil, e
	}
	parent, e := Canonical(filepath.Dir(abs))
	if e != nil {
		return nil, fmt.Errorf("resolve state directory %s: %w", filepath.Dir(abs), e)
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	r, e := os.OpenRoot(parent)
	if e != nil {
		return nil, fmt.Errorf("open state directory root %s: %w", parent, e)
	}
	fail := func(e error) (*Store, error) { r.Close(); return nil, e }
	for _, name := range []string{filepath.Base(abs), filepath.Base(abs) + ".lock"} {
		if st, e := r.Lstat(name); e == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
			return fail(fmt.Errorf("state/lock must be a regular file: %s", name))
		} else if e != nil && !os.IsNotExist(e) {
			return fail(e)
		}
	}
	f, e := r.OpenFile(filepath.Base(abs)+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return fail(e)
	}
	if e = lockFile(f); e != nil {
		f.Close()
		return fail(fmt.Errorf("batch already in use (lock %s.lock): %w; do not delete a live lock file", abs, e))
	}
	st, _ := f.Stat()
	stale := st != nil && st.Size() > 0
	host, _ := os.Hostname()
	b, _ := json.Marshal(struct {
		PID       int
		Host      string
		StartedAt time.Time
	}{os.Getpid(), host, time.Now().UTC()})
	if e = f.Truncate(0); e == nil {
		_, e = f.WriteAt(b, 0)
	}
	if e == nil {
		e = f.Sync()
	}
	if e != nil {
		unlockFile(f)
		f.Close()
		return fail(e)
	}
	return &Store{Path: abs, dir: r, lock: f, StaleLock: stale,
		createTemp: func(name string) (stateFile, error) { return r.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) },
		rename:     os.Rename, syncParent: SyncDir}, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	_ = s.lock.Truncate(0)
	_ = s.lock.Sync()
	_ = unlockFile(s.lock)
	e := s.lock.Close()
	s.lock = nil
	s.dir.Close()
	return e
}
func (s *Store) Exists() bool { _, e := s.dir.Lstat(filepath.Base(s.Path)); return !os.IsNotExist(e) }
func (s *Store) Load() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.dir.Open(filepath.Base(s.Path))
	if e != nil {
		return nil, fmt.Errorf("load batch state %s: %w", s.Path, e)
	}
	defer f.Close()
	return decode(f, s.Path)
}

func decode(r io.Reader, path string) (*State, error) {
	var state State
	var e error
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if e = d.Decode(&state); e != nil {
		return nil, fmt.Errorf("corrupt/incompatible JSON in %s: %w; restore a valid copy; the existing plan was not reset", path, e)
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return nil, fmt.Errorf("batch state %s contains trailing JSON; restore a valid copy", path)
	}
	if e = state.Validate(); e != nil {
		return nil, fmt.Errorf("invalid batch state %s: %w", path, e)
	}
	return &state, nil
}
func (s *Store) OpenLog() (*os.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := filepath.Base(s.Path) + ".log"
	if st, e := s.dir.Lstat(name); e == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("batch log must be a regular file")
	} else if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	return s.dir.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
}
func (s *Store) Save(state *State) error {
	return s.save(state, false)
}

// Initialize refuses an existing plan under the same exclusive application lock.
// External tools must also leave the destination alone while this lock is held.
func (s *Store) Initialize(state *State) error { return s.save(state, true) }
func (s *Store) save(state *State, initialize bool) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return fmt.Errorf("batch state store is closed")
	}
	if initialize {
		if _, e := s.dir.Lstat(filepath.Base(s.Path)); e == nil {
			return fmt.Errorf("batch state already exists at %s; resume it or choose another state path", s.Path)
		} else if !os.IsNotExist(e) {
			return fmt.Errorf("check batch state %s before initialization: %w", s.Path, e)
		}
	}
	state.UpdatedAt = time.Now().UTC()
	state.Recount()
	if e := state.Validate(); e != nil {
		return e
	}
	b, e := json.MarshalIndent(state, "", "  ")
	if e != nil {
		return e
	}
	name := "." + filepath.Base(s.Path) + "-" + ID() + ".tmp"
	f, e := s.createTemp(name)
	if e != nil {
		return fmt.Errorf("create temporary state beside %s: %w", s.Path, e)
	}
	defer func() {
		if e := s.dir.Remove(name); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, fmt.Errorf("remove temporary state %s: %w", name, e))
		}
	}()
	data := append(b, '\n')
	n, e := f.Write(data)
	if e == nil && n != len(data) {
		e = io.ErrShortWrite
	}
	if e != nil {
		e = fmt.Errorf("write temporary state: %w", e)
	}
	if e == nil {
		if se := f.Sync(); se != nil {
			e = fmt.Errorf("sync temporary state: %w", se)
		}
	}
	ce := f.Close()
	if ce != nil {
		e = errors.Join(e, fmt.Errorf("close temporary state: %w", ce))
	}
	if e != nil {
		return fmt.Errorf("save batch state %s: %w (previous plan retained)", s.Path, e)
	}
	if s.beforeReplace != nil {
		if e = s.beforeReplace(); e != nil {
			return e
		}
	}
	// A same-directory rename replaces the directory entry atomically on our
	// Unix targets. Do not unlink the previous plan, use linkat, or fall back to
	// truncating the destination. Both paths use the canonical parent directory.
	e = s.rename(filepath.Join(filepath.Dir(s.Path), name), s.Path)
	if e != nil {
		return fmt.Errorf("replace batch state: %w (previous plan retained)", e)
	}
	if e = s.syncParent(filepath.Dir(s.Path)); e != nil {
		return fmt.Errorf("sync state directory %s after replacement (new JSON published, durability unconfirmed): %w", filepath.Dir(s.Path), e)
	}
	return nil
}
