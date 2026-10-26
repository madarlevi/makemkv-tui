package makemkv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestHelperProcess(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != "helper" {
		return
	}
	fmt.Println("TCOUNT:1")
	fmt.Fprintln(os.Stderr, "MSG:1,0,0,\"stderr\",\"stderr\"")
	if os.Getenv("MAKEMKV_TEST_SLEEP") == "1" {
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}
func TestProcessCapturesBothPipes(t *testing.T) {
	t.Setenv("MAKEMKV_TEST_SLEEP", "0")
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	var lines []string
	e = (Process{exe}).Run(context.Background(), []string{"-test.run=TestHelperProcess", "--", "helper"}, func(s string) { lines = append(lines, s) })
	if e != nil || len(lines) != 2 {
		t.Fatal(lines, e)
	}
}
func TestProcessCancellation(t *testing.T) {
	t.Setenv("MAKEMKV_TEST_SLEEP", "1")
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var requested time.Time
	e = (Process{exe}).Run(ctx, []string{"-test.run=TestHelperProcess", "--", "helper"}, func(string) {
		if requested.IsZero() {
			requested = time.Now()
			cancel()
		}
	})
	if !errors.Is(e, context.Canceled) || requested.IsZero() || time.Since(requested) > 5*time.Second {
		t.Fatal(e)
	}
}
