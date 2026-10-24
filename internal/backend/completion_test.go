package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type completionRunner struct{ message string }

func (r completionRunner) Run(_ context.Context, args []string, emit func(string)) error {
	e := os.WriteFile(filepath.Join(args[len(args)-1], "engine.mkv"), []byte{0x1a, 0x45, 0xdf, 0xa3, 1}, 0600)
	if r.message != "" {
		emit(r.message)
	}
	return e
}
func TestBatchRequiresExplicitPositiveEngineSuccess(t *testing.T) {
	for _, tc := range []struct {
		message string
		ok      bool
	}{
		{"", false},
		{"MSG:5005,128,1,\"0 titles saved\",\"%1 titles saved\",\"0\"", false},
		{"MSG:5004,128,2,\"partial\",\"format\",\"1\",\"1\"", false},
		{"MSG:5005,128,1,\"1 titles saved\",\"%1 titles saved\",\"1\"", true},
	} {
		t.Run(tc.message, func(t *testing.T) {
			dir := t.TempDir()
			iso := filepath.Join(dir, "source.iso")
			os.WriteFile(iso, []byte("source"), 0600)
			m := MakeMKV{Runner: completionRunner{tc.message}}
			_, e := m.Extract(context.Background(), Request{RequireSuccess: true, Source: "iso:" + iso, Output: dir, IDs: []int{2}, Filenames: map[int]string{2: "title2.mkv"}}, func(Event) {})
			if (e == nil) != tc.ok {
				t.Fatal(tc, e)
			}
			if !tc.ok {
				if _, e = os.Stat(filepath.Join(dir, "title2.mkv")); !os.IsNotExist(e) {
					t.Fatal("unconfirmed output published")
				}
			}
		})
	}
}
