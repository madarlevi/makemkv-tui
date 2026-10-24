package backend

import (
	"context"
	"github.com/madarlevi/makemkv-tui/internal/makemkv"
	"github.com/madarlevi/makemkv-tui/internal/media"
)

type zeroLengthRunner struct{ makemkv.Executor }

func (r zeroLengthRunner) Run(ctx context.Context, args []string, emit func(string)) error {
	return r.Executor.Run(ctx, append([]string{"--minlength=0"}, args...), emit)
}
func (m MakeMKV) ScanBatch(ctx context.Context, source string, emit func(Event)) (*media.Disc, error) {
	m.Runner = zeroLengthRunner{m.Runner}
	return m.Scan(ctx, source, emit)
}
