package backend

import (
	"context"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/makemkv"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type MakeMKV struct{ Runner makemkv.Executor }

func (m MakeMKV) Capabilities() Capabilities {
	return Capabilities{Profiles: true, Description: "MakeMKV robot CLI: title selection; source track checkboxes unavailable. Optional post-extraction MKVToolNix track review."}
}
func (m MakeMKV) NormalizeSource(s string) (string, error) { return makemkv.Source(s) }
func convert(line string, i, id int, n int) Event {
	e := Event{Line: line, Index: i, TitleID: id}
	r, err := makemkv.Parse(line)
	if err != nil {
		return e
	}
	f := r.Fields
	switch r.Kind {
	case "MSG":
		flags, _ := strconv.Atoi(f[1])
		kind := "INFO"
		switch flags & 3854 {
		case 516, 1288:
			kind = "ERROR"
		case 1028:
			kind = "WARNING"
		}
		e.Line = kind + " · " + line
	case "PRGC", "PRGT":
		e.Operation = f[2]
	case "PRGV":
		a, _ := strconv.ParseFloat(f[0], 64)
		b, _ := strconv.ParseFloat(f[1], 64)
		c, _ := strconv.ParseFloat(f[2], 64)
		if c > 0 {
			e.Progress = true
			e.Current = max(0, min(1, a/c))
			e.Total = (float64(i) + max(0, min(1, b/c))) / float64(max(1, n))
		}
	}
	return e
}
func (m MakeMKV) Scan(ctx context.Context, s string, emit func(Event)) (*media.Disc, error) {
	var reported error
	d, e := makemkv.Info(ctx, m.Runner, s, func(s string) {
		emit(convert(s, 0, 0, 1))
		if err := reportedError(s); err != nil {
			reported = err
		}
	})
	if e == nil && reported != nil {
		e = reported
	}
	if e != nil {
		return nil, e
	}
	return d.Disc, nil
}
func (m MakeMKV) Drives(ctx context.Context, emit func(Event)) ([]Drive, error) {
	var out []Drive
	var parseErr error
	e := m.Runner.Run(ctx, []string{"-r", "--cache=1", "info", "disc:9999"}, func(s string) {
		emit(convert(s, 0, 0, 1))
		if !strings.HasPrefix(s, "DRV:") {
			return
		}
		r, e := makemkv.Parse(s)
		if e != nil {
			parseErr = e
			return
		}
		f := r.Fields
		if len(f) < 6 {
			parseErr = fmt.Errorf("invalid DRV record")
			return
		}
		i, e := strconv.Atoi(f[0])
		if e != nil {
			parseErr = e
			return
		}
		if f[4] != "" {
			d := Drive{Index: i, Name: f[4], Disc: f[5], State: f[1]}
			if len(f) > 6 {
				d.Device = f[6]
			}
			out = append(out, d)
		}
	})
	if e != nil {
		return nil, e
	}
	return out, parseErr
}
func (m MakeMKV) Extract(ctx context.Context, r Request, emit func(Event)) ([]Artifact, error) {
	if len(r.IDs) == 0 {
		return nil, fmt.Errorf("no titles selected")
	}
	if _, e := makemkv.Source(r.Source); e != nil {
		return nil, e
	}
	out, e := makemkv.Output(r.Output)
	if e != nil {
		return nil, e
	}
	seen := map[int]bool{}
	names := map[string]bool{}
	for _, id := range r.IDs {
		if id < 0 || seen[id] {
			return nil, fmt.Errorf("invalid or duplicate title ID %d", id)
		}
		seen[id] = true
		if name := r.Filenames[id]; name != "" {
			if e := preferences.ValidateName(name); e != nil {
				return nil, e
			}
			if names[name] {
				return nil, fmt.Errorf("duplicate output filename %s", name)
			}
			names[name] = true
			if _, e := os.Lstat(filepath.Join(out, name)); !os.IsNotExist(e) {
				return nil, fmt.Errorf("output already exists or cannot be checked: %s", name)
			}
		}
	}
	if r.Profile != "" {
		st, e := os.Stat(r.Profile)
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("profile must be a regular file")
		}
	}
	artifacts := []Artifact{}
	if r.StagedOnly && (!r.RequireSuccess || len(r.IDs) != 1) {
		return nil, fmt.Errorf("staged-only extraction requires one title and explicit engine success")
	}
	for i, id := range r.IDs {
		if e := ctx.Err(); e != nil {
			return artifacts, e
		}
		stage := out
		if !r.StagedOnly {
			stage, e = os.MkdirTemp(out, ".makemkv-tui-")
			if e != nil {
				return artifacts, e
			}
		} else {
			entries, err := os.ReadDir(stage)
			if err != nil {
				return artifacts, err
			}
			if len(entries) != 0 {
				return artifacts, fmt.Errorf("private batch attempt is not empty: %s; data retained", stage)
			}
		}
		emit(Event{Index: i, TitleID: id, Line: "Staging title in " + stage})
		args := []string{"-r", "--progress=-same"}
		if r.MinimumLength != nil {
			if *r.MinimumLength < 0 {
				return artifacts, fmt.Errorf("minimum title length cannot be negative")
			}
			args = append(args, "--minlength="+strconv.Itoa(*r.MinimumLength))
		}
		if r.Profile != "" {
			args = append(args, "--profile="+r.Profile)
		}
		args = append(args, "mkv", r.Source, strconv.Itoa(id), stage)
		var reported error
		confirmed := false
		e = m.Runner.Run(ctx, args, func(s string) {
			if rec, err := makemkv.Parse(s); err == nil {
				if rec.Kind == "MSG" && rec.Fields[0] == "5005" && len(rec.Fields) > 5 {
					saved, _ := strconv.Atoi(rec.Fields[5])
					confirmed = saved == 1
				}
			} else if r.RequireSuccess && strings.HasPrefix(s, "MSG:") {
				reported = fmt.Errorf("invalid engine completion record: %w", err)
			}
			emit(convert(s, i, id, len(r.IDs)))
			if err := reportedError(s); err != nil {
				reported = err
			}
		})
		if e == nil && reported != nil {
			e = reported
		}
		if e == nil && r.RequireSuccess && !confirmed {
			e = fmt.Errorf("MakeMKV exited without MSG:5005 success acknowledgement; staged output is not complete")
		}
		if e != nil {
			return artifacts, fmt.Errorf("%w; partial files retained in %s", e, stage)
		}
		entries, e := os.ReadDir(stage)
		if e != nil {
			return artifacts, e
		}
		files := []string{}
		for _, v := range entries {
			if !v.IsDir() && strings.EqualFold(filepath.Ext(v.Name()), ".mkv") {
				files = append(files, v.Name())
			}
		}
		if len(files) != 1 {
			return artifacts, fmt.Errorf("expected one MKV for title %d, found %d; files retained in %s", id, len(files), stage)
		}
		if r.StagedOnly {
			if e = WriteReceipt(stage, r.Source, id, files[0]); e != nil {
				return artifacts, fmt.Errorf("MakeMKV completed but engine-success receipt failed: %w; all output retained at %s", e, stage)
			}
			artifacts = append(artifacts, Artifact{id, filepath.Join(stage, files[0])})
			continue
		}
		name := r.Filenames[id]
		if name == "" {
			name = files[0]
		}
		target := filepath.Join(out, name)
		if e = PublishContext(ctx, filepath.Join(stage, files[0]), target); e != nil {
			return artifacts, e
		}
		artifacts = append(artifacts, Artifact{id, target})
		if e = os.RemoveAll(stage); e != nil {
			emit(Event{Index: i, TitleID: id, Line: "Warning: staging cleanup: " + e.Error()})
		}
	}
	return artifacts, nil
}

func reportedError(line string) error {
	r, e := makemkv.Parse(line)
	if e != nil || r.Kind != "MSG" {
		return nil
	}
	flags, _ := strconv.Atoi(r.Fields[1])
	code := r.Fields[0]
	if flags&3854 == 516 || flags&3854 == 1288 || code == "5004" || code == "5069" {
		return fmt.Errorf("MakeMKV MSG %s: %s", code, r.Fields[3])
	}
	return nil
}
