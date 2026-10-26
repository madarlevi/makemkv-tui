//go:build freebsd || linux || darwin

package makemkv

import (
	"context"
	"errors"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/proccontrol"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestIgnoringTermHelper(t *testing.T) {
	if os.Getenv("MAKEMKV_IGNORE_TERM_HELPER") != "1" {
		return
	}
	signal.Ignore(syscall.SIGTERM)
	fmt.Println("ready")
	time.Sleep(time.Minute)
	os.Exit(0)
}
func TestCancellationEscalatesAndSecondStopForces(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprint(force), func(t *testing.T) {
			t.Setenv("MAKEMKV_IGNORE_TERM_HELPER", "1")
			exe, e := os.Executable()
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			ctx, kill := proccontrol.WithForce(ctx)
			var start time.Time
			e = (Process{Binary: exe}).Run(ctx, []string{"-test.run=^TestIgnoringTermHelper$"}, func(line string) {
				if strings.TrimSpace(line) != "ready" || !start.IsZero() {
					return
				}
				start = time.Now()
				cancel()
				if force {
					kill()
				}
			})
			elapsed := time.Since(start)
			if !errors.Is(e, context.Canceled) || start.IsZero() {
				t.Fatal(e)
			}
			if elapsed > 5*time.Second {
				t.Fatal("termination did not escalate")
			}
			if force && elapsed > 1500*time.Millisecond {
				t.Fatal("second stop did not force promptly")
			}
			if !force && elapsed < 1800*time.Millisecond {
				t.Fatal("grace period was not honored")
			}
		})
	}
}
