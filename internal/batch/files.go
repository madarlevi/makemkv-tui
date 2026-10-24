package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/publish"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var errInvalidOutput = errors.New("invalid output data")

// checkPath rejects symlinks below a canonical root. os.Root additionally confines
// every read/write/link/remove, including if a component changes during access.
func checkPath(r *os.Root, rel string, missing bool) error {
	if !batchstate.Local(rel) {
		return fmt.Errorf("unsafe relative path %q", rel)
	}
	parts := strings.Split(filepath.FromSlash(rel), string(filepath.Separator))
	p := ""
	for _, part := range parts {
		p = filepath.Join(p, part)
		st, e := r.Lstat(p)
		if os.IsNotExist(e) && missing {
			return nil
		}
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed in batch path %s", rel)
		}
	}
	return nil
}
func ValidateSources(s *batchstate.State) error {
	for _, root := range []string{s.InputRoot, s.OutputRoot} {
		p, e := canonicalDir(root, false)
		if e != nil {
			return e
		}
		if p != root {
			return fmt.Errorf("batch root changed or became a symlink: %s", root)
		}
	}
	r, e := os.OpenRoot(s.InputRoot)
	if e != nil {
		return e
	}
	defer r.Close()
	for _, j := range s.ISOs {
		if e = checkPath(r, j.RelativePath, false); e != nil {
			return fmt.Errorf("source %s: %w", j.RelativePath, e)
		}
		st, e := r.Stat(filepath.FromSlash(j.RelativePath))
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() || st.Size() != j.Size || st.ModTime().UnixNano() != j.ModTimeNS {
			return fmt.Errorf("source changed since planning: %s; restore the planned ISO or create a new plan", j.RelativePath)
		}
	}
	if s.Profile != "" {
		h, e := profileHash(s.Profile)
		if e != nil {
			return e
		}
		if h != s.ProfileSHA256 {
			return fmt.Errorf("MakeMKV profile changed since planning: %s", s.Profile)
		}
	}
	return nil
}
func openOutput(s *batchstate.State) (*os.Root, error) {
	p, e := canonicalDir(s.OutputRoot, false)
	if e != nil {
		return nil, e
	}
	if p != s.OutputRoot {
		return nil, fmt.Errorf("output root changed")
	}
	return os.OpenRoot(p)
}
func verify(ctx context.Context, r *os.Root, rel string, syncFile bool) (*batchstate.Verification, error) {
	if e := checkPath(r, rel, false); e != nil {
		return nil, e
	}
	flags := os.O_RDONLY
	if syncFile {
		flags = os.O_RDWR
	}
	f, e := r.OpenFile(filepath.FromSlash(rel), flags, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !before.Mode().IsRegular() || before.Size() < 5 {
		return nil, fmt.Errorf("output is empty/non-regular: %s: %w", rel, errInvalidOutput)
	}
	header := make([]byte, 4)
	if _, e = io.ReadFull(f, header); e != nil {
		return nil, e
	}
	if !bytes.Equal(header, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return nil, fmt.Errorf("output lacks Matroska/EBML signature: %s: %w", rel, errInvalidOutput)
	}
	if _, e = f.Seek(0, 0); e != nil {
		return nil, e
	}
	h := sha256.New()
	buf := make([]byte, 1024*1024)
	for {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	after, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("output changed during verification: %s: %w", rel, errInvalidOutput)
	}
	if syncFile {
		if e = f.Sync(); e != nil {
			return nil, e
		}
	}
	return &batchstate.Verification{Size: after.Size(), SHA256: hex.EncodeToString(h.Sum(nil)), EBML: true, EngineSucceeded: true, ValidatedAt: time.Now().UTC()}, nil
}
func matches(v, w *batchstate.Verification) bool {
	return v != nil && w != nil && v.Size == w.Size && v.SHA256 == w.SHA256 && v.EBML && v.EngineSucceeded
}
func syncOutputDir(r *os.Root, rel string) error {
	for dir := filepath.Dir(filepath.FromSlash(rel)); ; dir = filepath.Dir(dir) {
		f, e := r.Open(dir)
		if e != nil {
			return e
		}
		if e = errors.Join(syncHandle(f), f.Close()); e != nil {
			return fmt.Errorf("sync output directory %s: %w", dir, e)
		}
		if dir == "." {
			return nil
		}
	}
}
func publishOutput(ctx context.Context, p publish.Publisher, s *batchstate.State, r *os.Root, i int, t *batchstate.Title) error {
	if e := checkPath(r, t.Output, true); e != nil {
		return e
	}
	if e := r.MkdirAll(filepath.Dir(filepath.FromSlash(t.Output)), 0755); e != nil {
		return e
	}
	if t.Verification == nil {
		return fmt.Errorf("cannot publish without durable verification")
	}
	return p.Publish(ctx, r, filepath.FromSlash(batchstate.Candidate(s, i, t)), filepath.FromSlash(t.Output), publish.Digest{Size: t.Verification.Size, SHA256: t.Verification.SHA256})
}
func cleanup(s *batchstate.State, r *os.Root, i int, t *batchstate.Title) error {
	if t.Status != batchstate.Completed || t.Verification == nil {
		return fmt.Errorf("refusing staging cleanup for an uncompleted/unverified title %d", t.ID)
	}
	if t.AttemptID == "" {
		return nil
	}
	rel := batchstate.StageDir(s, i, t)
	if e := checkPath(r, rel, true); e != nil {
		return e
	}
	return r.RemoveAll(filepath.FromSlash(rel))
}
