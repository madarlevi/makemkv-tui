package main

import (
	"context"
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/makemkv"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"github.com/madarlevi/makemkv-tui/internal/tui"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		return printVersion(false)
	}
	if len(os.Args) == 2 && os.Args[1] == "version" {
		return printVersion(false)
	}
	if len(os.Args) == 3 && os.Args[1] == "version" && os.Args[2] == "--json" {
		return printVersion(true)
	}
	if len(os.Args) > 1 && os.Args[1] == "batch" {
		return runBatch(os.Args[2:])
	}
	configDefault, e := preferences.Path()
	if e != nil {
		return e
	}
	config := flag.String("config", configDefault, "frontend preferences JSON")
	profile := flag.String("profile", "", "existing MakeMKV profile XML (selection behavior is engine/version-dependent)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: makemkv-tui [options] [source]\n       makemkv-tui batch INPUT --output OUTPUT\n       makemkv-tui batch --resume STATE [--retry-failed]\n       makemkv-tui batch --plans | --list | --state-dir\nSee batch --help for local plan storage and resume options.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 1 {
		return fmt.Errorf("usage: makemkv-tui [--config path] [--profile path] [ISO | folder | disc:N | dev:/device]")
	}
	prefs, e := preferences.Load(*config)
	if e != nil {
		return e
	}
	x, e := makemkv.Detect()
	if e != nil {
		return e
	}
	remux, _ := backend.DetectRemuxer()
	logDir := filepath.Join(filepath.Dir(*config), "logs")
	if e = os.MkdirAll(logDir, 0700); e != nil {
		return e
	}
	log, e := os.CreateTemp(logDir, time.Now().Format("20060102-150405")+"-*.log")
	if e != nil {
		return e
	}
	defer log.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	opts := tui.Options{Preferences: prefs, ConfigPath: *config, Profile: *profile, Log: log, LogPath: log.Name(), Remuxer: remux}
	_, e = tea.NewProgram(tui.NewWithOptions(ctx, backend.MakeMKV{Runner: x}, flag.Arg(0), opts), tea.WithAltScreen(), tea.WithoutSignalHandler()).Run()
	return e
}
