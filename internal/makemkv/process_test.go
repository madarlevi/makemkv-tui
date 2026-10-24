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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	e = (Process{exe}).Run(ctx, []string{"-test.run=TestHelperProcess", "--", "helper"}, func(string) { cancel() })
	if !errors.Is(e, context.Canceled) || time.Since(start) > 5*time.Second {
		t.Fatal(e)
	}
}
