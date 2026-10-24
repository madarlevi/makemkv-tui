package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/makemkv"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Track struct {
	ID         int    `json:"id"`
	Type       string `json:"type"`
	Codec      string `json:"codec"`
	Properties struct {
		Language     string `json:"language"`
		LanguageIETF string `json:"language_ietf"`
		Name         string `json:"track_name"`
		Default      bool   `json:"default_track"`
		Forced       bool   `json:"forced_track"`
	} `json:"properties"`
}
type Inventory struct {
	Tracks   []Track `json:"tracks"`
	Chapters []struct {
		Count int `json:"num_entries"`
	} `json:"chapters"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}
type Remuxer interface {
	Identify(context.Context, string) (Inventory, error)
	Remux(context.Context, string, string, Inventory, map[int]bool, func(Event)) (string, error)
}
type MKVToolNix struct{ Runner makemkv.Executor }

func DetectRemuxer() (Remuxer, error) {
	p, e := exec.LookPath("mkvmerge")
	if e != nil {
		return nil, fmt.Errorf("install MKVToolNix and put mkvmerge on PATH to review extracted audio/subtitles")
	}
	return MKVToolNix{makemkv.Process{Binary: p}}, nil
}
func warningOnly(e error) bool { var x *exec.ExitError; return errors.As(e, &x) && x.ExitCode() == 1 }
func (m MKVToolNix) Identify(ctx context.Context, path string) (Inventory, error) {
	var b strings.Builder
	e := m.Runner.Run(ctx, []string{"-J", path}, func(s string) { b.WriteString(s); b.WriteByte('\n') })
	var inv Inventory
	if e != nil && !warningOnly(e) {
		return inv, e
	}
	if e = json.Unmarshal([]byte(b.String()), &inv); e != nil {
		return inv, fmt.Errorf("mkvmerge identification: %w", e)
	}
	if len(inv.Errors) > 0 {
		return inv, fmt.Errorf("mkvmerge: %s", strings.Join(inv.Errors, "; "))
	}
	seen := map[int]bool{}
	for _, t := range inv.Tracks {
		if t.ID < 0 || seen[t.ID] {
			return inv, fmt.Errorf("invalid or duplicate MKV track ID")
		}
		seen[t.ID] = true
	}
	if len(inv.Tracks) == 0 {
		return inv, fmt.Errorf("no tracks identified")
	}
	return inv, nil
}
func RemuxArgs(input, output string, inv Inventory, selected map[int]bool) ([]string, error) {
	known := map[int]bool{}
	audio, subs := []string{}, []string{}
	hasVideo := false
	for _, t := range inv.Tracks {
		known[t.ID] = true
		switch t.Type {
		case "video":
			hasVideo = true
		case "audio":
			if selected[t.ID] {
				audio = append(audio, strconv.Itoa(t.ID))
			}
		case "subtitles":
			if selected[t.ID] {
				subs = append(subs, strconv.Itoa(t.ID))
			}
		}
	}
	for id := range selected {
		if !known[id] {
			return nil, fmt.Errorf("unknown output track ID %d", id)
		}
	}
	if !hasVideo && len(audio) == 0 {
		return nil, fmt.Errorf("keep at least one video or audio track")
	}
	args := []string{"-o", output}
	if len(audio) == 0 {
		args = append(args, "--no-audio")
	} else {
		args = append(args, "--audio-tracks", strings.Join(audio, ","))
	}
	if len(subs) == 0 {
		args = append(args, "--no-subtitles")
	} else {
		args = append(args, "--subtitle-tracks", strings.Join(subs, ","))
	}
	return append(args, input), nil
}
func (m MKVToolNix) Remux(ctx context.Context, input, name string, inv Inventory, selected map[int]bool, emit func(Event)) (string, error) {
	if e := preferences.ValidateName(name); e != nil {
		return "", e
	}
	abs, e := filepath.Abs(input)
	if e != nil {
		return "", e
	}
	target := filepath.Join(filepath.Dir(abs), name)
	if _, e = os.Lstat(target); !os.IsNotExist(e) {
		return "", fmt.Errorf("output exists or cannot be checked: %s", target)
	}
	stage, e := os.MkdirTemp(filepath.Dir(abs), ".makemkv-remux-")
	if e != nil {
		return "", e
	}
	temp := filepath.Join(stage, "selected.mkv")
	args, e := RemuxArgs(abs, temp, inv, selected)
	if e != nil {
		os.Remove(stage)
		return "", e
	}
	e = m.Runner.Run(ctx, args, func(s string) { emit(Event{Line: s}) })
	if ctx.Err() != nil {
		return "", fmt.Errorf("%w; partial remux retained in %s", ctx.Err(), stage)
	}
	if e != nil && !warningOnly(e) {
		return "", fmt.Errorf("%w; partial remux retained in %s", e, stage)
	}
	if e != nil {
		emit(Event{Line: "Warning: mkvmerge completed with warnings; inspect logs"})
	}
	if e = PublishContext(ctx, temp, target); e != nil {
		return "", e
	}
	if e = os.RemoveAll(stage); e != nil {
		emit(Event{Line: "Warning: staging cleanup: " + e.Error()})
	}
	return target, nil
}
