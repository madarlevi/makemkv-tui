package makemkv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fake struct {
	calls [][]string
	fail  error
}

func (f *fake) Run(ctx context.Context, args []string, emit func(string)) error {
	f.calls = append(f.calls, append([]string(nil), args...))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	emit("TCOUNT:1")
	emit("TINFO:7,2,0,\"Feature\"")
	return f.fail
}
func TestRunner(t *testing.T) {
	f := &fake{}
	d, e := Info(context.Background(), f, "iso:/movie with spaces.iso", func(string) {})
	if e != nil || d.Sorted()[0].ID != 7 {
		t.Fatal(d, e)
	}
	e = Extract(context.Background(), f, "iso:/movie with spaces.iso", "/out folder", []int{7, 12}, func(int, int, string) {})
	if e != nil {
		t.Fatal(e)
	}
	want := []string{"-r", "--progress=-same", "mkv", "iso:/movie with spaces.iso", "12", "/out folder"}
	if !reflect.DeepEqual(f.calls[2], want) {
		t.Fatal(f.calls)
	}
}
func TestStopOnFailureAndCancel(t *testing.T) {
	f := &fake{fail: errors.New("failed")}
	if Extract(context.Background(), f, "disc:0", ".", []int{0, 1}, func(int, int, string) {}) == nil || len(f.calls) != 1 {
		t.Fatal(f.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f = &fake{}
	if !errors.Is(Extract(ctx, f, "disc:0", ".", []int{0}, func(int, int, string) {}), context.Canceled) || len(f.calls) != 0 {
		t.Fatal("cancel ignored")
	}
}
func TestSources(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "movie one.ISO")
	if e := os.WriteFile(p, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := Source(p)
	if e != nil || s != "iso:"+p {
		t.Fatal(s, e)
	}
	if _, e = Source("disc:no"); e == nil {
		t.Fatal("invalid drive")
	}
	if _, e = Source(filepath.Join(dir, "missing.iso")); e == nil {
		t.Fatal("missing file")
	}
	paths, e := DiscoverISOs(dir)
	if e != nil || len(paths) != 1 {
		t.Fatal(paths, e)
	}
	if _, e = Output(filepath.Join(dir, "new output")); e != nil {
		t.Fatal(e)
	}
}
