// Package proccontrol carries an optional second-stop force request to a process runner.
package proccontrol

import (
	"context"
	"sync"
)

type key struct{}

func WithForce(ctx context.Context) (context.Context, func()) {
	ch := make(chan struct{})
	var once sync.Once
	return context.WithValue(ctx, key{}, (<-chan struct{})(ch)), func() { once.Do(func() { close(ch) }) }
}
func Force(ctx context.Context) <-chan struct{} {
	ch, _ := ctx.Value(key{}).(<-chan struct{})
	return ch
}
