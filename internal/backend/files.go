package backend

import (
	"context"
	"github.com/madarlevi/makemkv-tui/internal/publish"
	"os"
	"path/filepath"
	"strings"
)

type Entry struct {
	Path, Name string
	Directory  bool
}

func Browse(dir string) ([]Entry, error) {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return nil, e
	}
	out := []Entry{}
	for _, v := range entries {
		p := filepath.Join(dir, v.Name())
		st, e := os.Stat(p)
		if e != nil {
			continue
		}
		if st.IsDir() || strings.EqualFold(filepath.Ext(p), ".iso") {
			out = append(out, Entry{p, v.Name(), st.IsDir()})
		}
	}
	return out, nil
}

// Publish uses a native no-replace rename or a verified exclusive-copy fallback.
// The fallback is non-atomic at the final filename; staging survives failure.
func Publish(staged, target string) error {
	return PublishContext(context.Background(), staged, target)
}
func PublishContext(ctx context.Context, staged, target string) error {
	return publish.Artifact(ctx, staged, target)
}
