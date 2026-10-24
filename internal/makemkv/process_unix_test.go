//go:build freebsd || linux || darwin

package makemkv

import (
	"context"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/proccontrol"
	"os"
	"os/signal"
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
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx, kill := proccontrol.WithForce(ctx)
			start := time.Now()
			e = (Process{Binary: exe}).Run(ctx, []string{"-test.run=^TestIgnoringTermHelper$"}, func(string) {
				cancel()
				if force {
					kill()
				}
			})
			elapsed := time.Since(start)
			if e != context.Canceled {
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
