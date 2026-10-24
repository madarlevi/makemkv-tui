package makemkv

import (
	"bufio"
	"context"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/proccontrol"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Executor interface {
	Run(context.Context, []string, func(string)) error
}
type Process struct{ Binary string }

func Detect() (Process, error) {
	p, e := exec.LookPath("makemkvcon")
	if e != nil {
		return Process{}, fmt.Errorf("makemkvcon was not found on PATH: install MakeMKV and add its executable directory to PATH (FreeBSD: check multimedia/makemkv)")
	}
	return Process{p}, nil
}
func (p Process) Run(ctx context.Context, args []string, emit func(string)) error {
	cmd := exec.CommandContext(ctx, p.Binary, args...)
	configure(cmd)
	cmd.WaitDelay = 3 * time.Second
	rd, wr, e := os.Pipe()
	if e != nil {
		return e
	}
	defer rd.Close()
	cmd.Stdout = wr
	cmd.Stderr = wr
	if e = cmd.Start(); e != nil {
		wr.Close()
		return fmt.Errorf("start MakeMKV: %w", e)
	}
	processDone := make(chan struct{})
	go func() {
		select {
		case <-proccontrol.Force(ctx):
			_ = forceProcess(cmd)
		case <-ctx.Done():
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
				_ = forceProcess(cmd)
			case <-proccontrol.Force(ctx):
				_ = forceProcess(cmd)
			case <-processDone:
			}
		case <-processDone:
		}
	}()
	wr.Close()
	var scanErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s := bufio.NewScanner(rd)
		s.Buffer(make([]byte, 65536), 4*1024*1024)
		for s.Scan() {
			emit(s.Text())
		}
		scanErr = s.Err()
		if scanErr != nil {
			_ = forceProcess(cmd)
		}
	}()
	e = cmd.Wait()
	if ctx.Err() != nil {
		_ = forceProcess(cmd)
	} // reap any surviving members of the cancelled process group
	close(processDone)
	timer := time.AfterFunc(3*time.Second, func() { rd.Close() })
	wg.Wait()
	timer.Stop()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e != nil {
		return fmt.Errorf("%s failed: %w (check session logs)", filepath.Base(p.Binary), e)
	}
	return scanErr
}
func Info(ctx context.Context, x Executor, source string, emit func(string)) (*Disc, error) {
	d := NewDisc()
	var parseErr error
	e := x.Run(ctx, []string{"-r", "info", source}, func(line string) {
		emit(line)
		r, err := Parse(line)
		if err != nil {
			if recognized(line) {
				parseErr = err
			}
			return
		}
		d.Apply(r)
	})
	if e != nil {
		return nil, e
	}
	if parseErr != nil {
		return nil, fmt.Errorf("invalid robot metadata: %w", parseErr)
	}
	if len(d.Titles) == 0 {
		return nil, fmt.Errorf("no titles found: check source, MakeMKV license, media and permissions")
	}
	return d, nil
}
func recognized(s string) bool {
	for _, k := range []string{"TINFO:", "SINFO:", "CINFO:", "TCOUNT:", "MSG:"} {
		if strings.HasPrefix(s, k) {
			return true
		}
	}
	return false
}
func Extract(ctx context.Context, x Executor, source, out string, ids []int, emit func(int, int, string)) error {
	for i, id := range ids {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := x.Run(ctx, []string{"-r", "--progress=-same", "mkv", source, strconv.Itoa(id), out}, func(s string) { emit(i, id, s) }); e != nil {
			return e
		}
	}
	return nil
}
func Source(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("enter an ISO, disc folder, disc:N or dev:/device")
	}
	if strings.HasPrefix(s, "disc:") {
		n, e := strconv.Atoi(strings.TrimPrefix(s, "disc:"))
		if e != nil || n < 0 {
			return "", fmt.Errorf("drive source must be disc:N with N >= 0")
		}
		return s, nil
	}
	if strings.HasPrefix(s, "dev:") {
		if len(s) == 4 {
			return "", fmt.Errorf("device path is empty")
		}
		return s, nil
	}
	prefix := ""
	for _, p := range []string{"iso:", "file:"} {
		if strings.HasPrefix(s, p) {
			prefix = p
			s = strings.TrimPrefix(s, p)
			break
		}
	}
	p, e := filepath.Abs(s)
	if e != nil {
		return "", e
	}
	st, e := os.Stat(p)
	if e != nil {
		return "", fmt.Errorf("source: %w", e)
	}
	if st.Mode()&os.ModeDevice != 0 {
		return "dev:" + p, nil
	}
	if st.IsDir() {
		if prefix == "iso:" {
			return "", fmt.Errorf("ISO source must be a file")
		}
		return "file:" + p, nil
	}
	if !st.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(p), ".iso") {
		return "", fmt.Errorf("choose an ISO file or Blu-ray/DVD folder")
	}
	return "iso:" + p, nil
}
func Output(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("output folder is required")
	}
	p, e := filepath.Abs(s)
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(p, 0755); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(p, ".makemkv-tui-*")
	if e != nil {
		return "", fmt.Errorf("output is not writable: %w", e)
	}
	name := f.Name()
	f.Close()
	e = os.Remove(name)
	return p, e
}

// DiscoverISOs searches one directory, keeping discovery inexpensive and predictable.
func DiscoverISOs(dir string) ([]string, error) {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return nil, e
	}
	var out []string
	for _, v := range entries {
		if !v.IsDir() && strings.EqualFold(filepath.Ext(v.Name()), ".iso") {
			p, e := filepath.Abs(filepath.Join(dir, v.Name()))
			if e != nil {
				return nil, e
			}
			out = append(out, p)
		}
	}
	return out, nil
}
