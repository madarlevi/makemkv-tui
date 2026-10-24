// Package publish finalizes files without hard links or replacing destinations.
package publish

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrNoReplace = errors.New("atomic no-replace rename unavailable")
var errInvalidFile = errors.New("empty/non-regular publication file")

type Digest struct {
	Size   int64
	SHA256 string
}
type File interface {
	io.Writer
	Sync() error
	Close() error
}

// Hooks are per-instance, allowing disk/FUSE faults without global mutations.
type Publisher struct {
	Rename  func(*os.File, string, *os.File, string) error
	Create  func(*os.Root, string) (File, error)
	Copy    func(context.Context, io.Writer, io.Reader) error
	SyncDir func(*os.File) error
}

func local(path string) bool {
	return filepath.IsLocal(path) && filepath.Clean(path) == path && path != "."
}

func Hash(ctx context.Context, root *os.Root, path string, sync bool) (digest Digest, err error) {
	if !local(path) {
		return digest, fmt.Errorf("unsafe publication path %q", path)
	}
	st, e := root.Lstat(path)
	if e != nil {
		return digest, e
	}
	if !st.Mode().IsRegular() || st.Size() == 0 {
		return digest, fmt.Errorf("%w: %s", errInvalidFile, path)
	}
	flags := os.O_RDONLY
	if sync {
		flags = os.O_RDWR
	}
	f, e := root.OpenFile(path, flags, 0)
	if e != nil {
		return digest, e
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	before, e := f.Stat()
	if e != nil {
		return digest, e
	}
	if !os.SameFile(st, before) {
		return digest, fmt.Errorf("publication file changed while opening: %s", path)
	}
	h := sha256.New()
	if e = copyContext(ctx, h, f); e != nil {
		return digest, e
	}
	after, e := f.Stat()
	if e != nil {
		return digest, e
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return digest, fmt.Errorf("publication file changed while hashing: %s", path)
	}
	if sync {
		if e = f.Sync(); e != nil {
			return digest, e
		}
	}
	return Digest{after.Size(), hex.EncodeToString(h.Sum(nil))}, nil
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, 1024*1024)
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		n, e := src.Read(buf)
		if n > 0 {
			written, we := dst.Write(buf[:n])
			if we != nil {
				return we
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
	}
}

func (p Publisher) sync(root *os.Root, path string) error {
	fn := p.SyncDir
	if fn == nil {
		fn = syncDirectory
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		f, e := root.Open(dir)
		if e != nil {
			return e
		}
		if e = errors.Join(fn(f), f.Close()); e != nil {
			return fmt.Errorf("sync directory %s: %w", dir, e)
		}
		if dir == "." {
			return nil
		}
	}
}

func (p Publisher) copyFile(ctx context.Context, r *os.Root, source, target string) error {
	input, e := r.Open(source)
	if e != nil {
		return e
	}
	create := p.Create
	if create == nil {
		create = func(root *os.Root, path string) (File, error) {
			return root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		}
	}
	output, e := create(r, target)
	if e != nil {
		return errors.Join(e, input.Close())
	}
	copyFn := p.Copy
	if copyFn == nil {
		copyFn = copyContext
	}
	e = copyFn(ctx, output, input)
	if e == nil {
		e = output.Sync()
	}
	// Retain even incomplete destinations; no unlink-by-name race can remove
	// an unrelated file another program substituted after creation.
	return errors.Join(e, output.Close(), input.Close())
}

// Publish never removes staging. Rename may consume the source file, but the
// staging directory/receipts survive until the caller commits its JSON state.
// The copy fallback is deliberately NOT atomic at the final filename.
func (p Publisher) Publish(ctx context.Context, r *os.Root, source, target string, want Digest) (err error) {
	if !local(source) || !local(target) || source == target {
		return fmt.Errorf("unsafe publication paths %q -> %q", source, target)
	}
	defer func() {
		if err != nil {
			err = fmt.Errorf("finalize %s -> %s: %w; staging and any final/temporary files retained; inspect these paths before retrying", filepath.Join(r.Name(), source), filepath.Join(r.Name(), target), err)
		}
	}()
	if e := ctx.Err(); e != nil {
		return e
	}
	if _, e := r.Lstat(target); e == nil {
		return fmt.Errorf("output collision: %s: %w", target, os.ErrExist)
	} else if !os.IsNotExist(e) {
		return e
	}
	if e := r.MkdirAll(filepath.Dir(target), 0755); e != nil {
		return e
	}
	got, e := Hash(ctx, r, source, true)
	if e != nil {
		return e
	}
	if want.Size == 0 {
		want = got
	}
	if got != want {
		return fmt.Errorf("staging size/SHA-256 mismatch at %s", source)
	}
	oldDir, e := r.Open(filepath.Dir(source))
	if e != nil {
		return e
	}
	defer oldDir.Close()
	newDir, e := r.Open(filepath.Dir(target))
	if e != nil {
		return e
	}
	defer newDir.Close()
	rename := p.Rename
	if rename == nil {
		rename = renameNoReplace
	}
	e = rename(oldDir, filepath.Base(source), newDir, filepath.Base(target))
	if e != nil && !unavailable(e) {
		return fmt.Errorf("no-replace rename: %w", e)
	}
	if e != nil {
		// Prepare a unique, durable temporary copy in the private staging folder.
		// This can require two additional file-sized allocations. No hard-link
		// fallback or unchecked rename is used, even when the mount supports ln.
		temporary, te := preparedCopy(ctx, r, filepath.Dir(source), want)
		if te != nil {
			return te
		}
		if temporary == "" {
			tmp, te := createTemp()
			if te != nil {
				return te
			}
			temporary = filepath.Join(filepath.Dir(source), tmp)
			if ce := p.copyFile(ctx, r, source, temporary); ce != nil {
				return errors.Join(fmt.Errorf("rename unavailable: %w", e), fmt.Errorf("prepare temporary copy %s: %w", temporary, ce))
			}
			if v, ve := Hash(ctx, r, temporary, true); ve != nil || v != want {
				return errors.Join(ve, fmt.Errorf("temporary copy %s failed verification", temporary))
			}
		}
		if se := p.sync(r, temporary); se != nil {
			return se
		}
		// O_EXCL is the final race barrier. A target appearing after the early
		// check is an error, including symlinks, directories and empty files.
		if ce := p.copyFile(ctx, r, temporary, target); ce != nil {
			return errors.Join(fmt.Errorf("rename unavailable: %w", e), fmt.Errorf("exclusive final copy (possibly incomplete, never overwritten): %w", ce))
		}
	}
	final, e := Hash(ctx, r, target, true)
	if e != nil {
		return e
	}
	if final != want {
		return fmt.Errorf("final output size/SHA-256 mismatch: %s", target)
	}
	if e = p.sync(r, target); e != nil {
		return fmt.Errorf("sync final directory: %w", e)
	}
	if e = p.sync(r, source); e != nil {
		return fmt.Errorf("sync staging directory: %w", e)
	}
	return nil
}

func preparedCopy(ctx context.Context, r *os.Root, parent string, want Digest) (string, error) {
	dir, e := r.Open(parent)
	if e != nil {
		return "", e
	}
	list, e := dir.ReadDir(-1)
	e = errors.Join(e, dir.Close())
	if e != nil {
		return "", e
	}
	for _, entry := range list {
		if !strings.HasPrefix(entry.Name(), ".publish-") || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		// Never replace/delete a failed temporary file; a complete matching
		// copy can be reused instead of allocating another multi-gigabyte file.
		v, e := Hash(ctx, r, path, false)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if e != nil && !errors.Is(e, errInvalidFile) && !errors.Is(e, os.ErrNotExist) {
			return "", e
		}
		if e == nil && v == want {
			if _, e = Hash(ctx, r, path, true); e != nil {
				return "", e
			}
			return path, nil
		}
	}
	return "", nil
}

func createTemp() (string, error) {
	nonce := make([]byte, 16)
	if _, e := rand.Read(nonce); e != nil {
		return "", e
	}
	return ".publish-" + hex.EncodeToString(nonce) + ".tmp", nil
}

// Artifact uses the target parent as a confinement root for shared adapters.
func Artifact(ctx context.Context, source, target string) error {
	source, e := filepath.Abs(source)
	if e != nil {
		return e
	}
	target, e = filepath.Abs(target)
	if e != nil {
		return e
	}
	r, e := os.OpenRoot(filepath.Dir(target))
	if e != nil {
		return e
	}
	defer r.Close()
	rel, e := filepath.Rel(filepath.Dir(target), source)
	if e != nil {
		return e
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("staging is outside output directory: %s", source)
	}
	return (Publisher{}).Publish(ctx, r, rel, filepath.Base(target), Digest{})
}
