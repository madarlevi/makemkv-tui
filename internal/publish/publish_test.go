package publish

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func setup(t *testing.T) (*os.Root, string, string, []byte) {
	t.Helper()
	r, e := os.OpenRoot(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	source := filepath.Join("attempt 季節", "title_t00.mkv")
	target := filepath.Join("done with spaces", "title7.mkv")
	data := append([]byte{0x1a, 0x45, 0xdf, 0xa3}, bytes.Repeat([]byte("movie data"), 1000)...)
	if e = r.MkdirAll(filepath.Dir(source), 0700); e != nil {
		t.Fatal(e)
	}
	f, e := r.OpenFile(source, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(data); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	return r, source, target, data
}
func read(t *testing.T, r *os.Root, path string) []byte {
	t.Helper()
	f, e := r.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	b, e := io.ReadAll(f)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func unsupported(*os.File, string, *os.File, string) error { return ErrNoReplace }

func TestFUSEFallbackWithoutHardLinks(t *testing.T) {
	r, source, target, data := setup(t)
	copies := 0
	p := Publisher{Rename: unsupported, Copy: func(ctx context.Context, w io.Writer, rd io.Reader) error { copies++; return copyContext(ctx, w, rd) }}
	if e := p.Publish(context.Background(), r, source, target, Digest{}); e != nil {
		t.Fatal(e)
	}
	if copies != 2 || !bytes.Equal(read(t, r, source), data) || !bytes.Equal(read(t, r, target), data) {
		t.Fatal("copy publication lost data")
	}
	if e := p.Publish(context.Background(), r, source, target, Digest{}); !errors.Is(e, os.ErrExist) {
		t.Fatal("existing output not rejected", e)
	}
}

func TestDestinationAppearingDuringPublicationIsNeverOverwritten(t *testing.T) {
	r, source, target, data := setup(t)
	p := Publisher{Rename: func(*os.File, string, *os.File, string) error {
		f, e := r.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write([]byte("other program's file"))
		return errors.Join(e, f.Close(), ErrNoReplace)
	}}
	e := p.Publish(context.Background(), r, source, target, Digest{})
	if !errors.Is(e, os.ErrExist) || string(read(t, r, target)) != "other program's file" || !bytes.Equal(read(t, r, source), data) {
		t.Fatal("collision lost data", e)
	}
}

func TestRenameEIOIsNotSilentlyReinterpretedAsUnsupported(t *testing.T) {
	r, source, target, data := setup(t)
	created := false
	p := Publisher{Rename: func(*os.File, string, *os.File, string) error {
		return &os.LinkError{Op: "renameat2", Old: source, New: target, Err: syscall.EIO}
	}, Create: func(*os.Root, string) (File, error) { created = true; return nil, errors.New("unexpected") }}
	e := p.Publish(context.Background(), r, source, target, Digest{})
	if !errors.Is(e, syscall.EIO) || created || !strings.Contains(e.Error(), source) || !bytes.Equal(read(t, r, source), data) {
		t.Fatal(e)
	}
}

type faultyFile struct {
	File
	fault string
}

func (f faultyFile) Sync() error {
	if f.fault == "sync" {
		return syscall.EIO
	}
	return f.File.Sync()
}
func (f faultyFile) Close() error {
	e := f.File.Close()
	if f.fault == "close" {
		return errors.Join(e, syscall.EIO)
	}
	return e
}

func TestCopyWriteSyncCloseAndValidationFailuresRetainStaging(t *testing.T) {
	for _, fault := range []string{"temporary write", "final write", "sync", "close", "validation", "directory sync"} {
		t.Run(fault, func(t *testing.T) {
			r, source, target, data := setup(t)
			copies := 0
			p := Publisher{Rename: unsupported}
			p.Create = func(root *os.Root, path string) (File, error) {
				f, e := root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if e != nil {
					return nil, e
				}
				if path == target {
					return faultyFile{f, fault}, nil
				}
				return f, nil
			}
			p.Copy = func(ctx context.Context, w io.Writer, rd io.Reader) error {
				copies++
				if (fault == "temporary write" && copies == 1) || (fault == "final write" && copies == 2) {
					buf := make([]byte, 15)
					_, _ = rd.Read(buf)
					_, _ = w.Write(buf)
					return syscall.EIO
				}
				if fault == "validation" && copies == 2 {
					_, e := w.Write([]byte("corrupt output"))
					return e
				}
				return copyContext(ctx, w, rd)
			}
			if fault == "directory sync" {
				p.SyncDir = func(*os.File) error { return syscall.EIO }
			}
			e := p.Publish(context.Background(), r, source, target, Digest{})
			if e == nil || !bytes.Equal(read(t, r, source), data) || !strings.Contains(e.Error(), "retained") {
				t.Fatal("failure destroyed source", e)
			}
			if fault == "final write" {
				if len(read(t, r, target)) != 15 {
					t.Fatal("partial final should remain isolated from complete source")
				}
			}
		})
	}
}

func TestCancelledFinalCopyRetainsOriginal(t *testing.T) {
	r, source, target, data := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	copies := 0
	p := Publisher{Rename: unsupported, Copy: func(ctx context.Context, w io.Writer, rd io.Reader) error {
		copies++
		if copies == 2 {
			cancel()
		}
		return copyContext(ctx, w, rd)
	}}
	if e := p.Publish(ctx, r, source, target, Digest{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if !bytes.Equal(read(t, r, source), data) {
		t.Fatal("cancel deleted complete MKV")
	}
}

func TestNativeNoReplaceRace(t *testing.T) {
	r, source, target, data := setup(t)
	p := Publisher{Rename: func(a *os.File, from string, b *os.File, to string) error {
		f, e := r.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write([]byte("existing"))
		if e = errors.Join(e, f.Close()); e != nil {
			return e
		}
		return renameNoReplace(a, from, b, to)
	}}
	if e := p.Publish(context.Background(), r, source, target, Digest{}); !errors.Is(e, os.ErrExist) {
		t.Fatal(e)
	}
	if string(read(t, r, target)) != "existing" || !bytes.Equal(read(t, r, source), data) {
		t.Fatal("native race overwrote destination")
	}
}

func TestPreparedTemporaryCopyIsReusedAfterFailure(t *testing.T) {
	r, source, target, data := setup(t)
	p := Publisher{Rename: unsupported, Create: func(root *os.Root, path string) (File, error) {
		if path == target {
			return nil, syscall.EIO
		}
		return root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	}}
	if e := p.Publish(context.Background(), r, source, target, Digest{}); !errors.Is(e, syscall.EIO) {
		t.Fatal(e)
	}
	copies := 0
	p = Publisher{Rename: unsupported, Copy: func(ctx context.Context, w io.Writer, rd io.Reader) error { copies++; return copyContext(ctx, w, rd) }}
	if e := p.Publish(context.Background(), r, source, target, Digest{}); e != nil {
		t.Fatal(e)
	}
	if copies != 1 || !bytes.Equal(read(t, r, target), data) || !bytes.Equal(read(t, r, source), data) {
		t.Fatal("prepared copy was not reused", copies)
	}
}
