package backend

import (
	"context"
	"strings"
	"testing"
)

func TestBatchScanFixesMinimumLengthWithoutChangingSingleScan(t *testing.T) {
	f := &engineFake{}
	m := MakeMKV{Runner: f}
	if _, e := m.ScanBatch(context.Background(), "iso:/source.iso", func(Event) {}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(strings.Join(f.calls[0], " "), "--minlength=0") {
		t.Fatal(f.calls)
	}
	if _, e := m.Scan(context.Background(), "iso:/source.iso", func(Event) {}); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(strings.Join(f.calls[1], " "), "--minlength") {
		t.Fatal("single source behavior changed")
	}
}
