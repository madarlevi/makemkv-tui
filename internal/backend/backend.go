// Package backend defines the boundary between the TUI and processing engines.
package backend

import (
	"context"
	"github.com/madarlevi/makemkv-tui/internal/media"
)

type Capabilities struct {
	IndividualSourceTracks bool
	ChapterTimestamps      bool
	Profiles               bool
	Description            string
}
type Event struct {
	Line           string
	Index, TitleID int
	Current, Total float64
	Progress       bool
	Operation      string
}
type Artifact struct {
	TitleID int
	Path    string
}
type Request struct {
	MinimumLength  *int // nil preserves engine preferences; batch fixes filtering to zero.
	RequireSuccess bool // Require an explicit engine success acknowledgement (batch safety).
	StagedOnly     bool // Batch owns finalization/cleanup; leave the actual engine filename intact.
	Source, Output string
	IDs            []int
	Filenames      map[int]string
	Profile        string
}
type Drive struct {
	Index         int
	Name, Disc    string
	State, Device string
}
type Engine interface {
	Capabilities() Capabilities
	NormalizeSource(string) (string, error)
	Scan(context.Context, string, func(Event)) (*media.Disc, error)
	Drives(context.Context, func(Event)) ([]Drive, error)
	Extract(context.Context, Request, func(Event)) ([]Artifact, error)
}
type BatchScanner interface {
	ScanBatch(context.Context, string, func(Event)) (*media.Disc, error)
}
