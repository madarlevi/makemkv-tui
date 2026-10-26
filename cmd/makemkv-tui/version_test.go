package main

import (
	"io"
	"os"
	"runtime/debug"
	"strings"
	"testing"
)

func TestModuleInstallVersionAndLinkedIdentity(t *testing.T) {
	build := &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: strings.Repeat("b", 40)}}}
	tag, commit := resolveVersion("dev", "unknown", "", build)
	if tag != "v1.2.3" || commit != strings.Repeat("b", 40) {
		t.Fatal("module installation lost its version", tag, commit)
	}
	tag, commit = resolveVersion("dev", "unknown", "MKVTUI_BUILD_V1|v9.8.7|"+strings.Repeat("a", 40)+"|END_MKVTUI_BUILD_V1", build)
	if tag != "v9.8.7" || commit != strings.Repeat("a", 40) {
		t.Fatal("module version overrode linked release identity")
	}
}

func TestVersionCommandsNeedNoEngineOrConfiguration(t *testing.T) {
	oldVersion, oldRevision, oldArgs, oldIdentity := version, revision, os.Args, releaseIdentity
	t.Cleanup(func() { version, revision, os.Args, releaseIdentity = oldVersion, oldRevision, oldArgs, oldIdentity })
	version = "v9.8.7"
	revision = strings.Repeat("a", 40)
	releaseIdentity = "MKVTUI_BUILD_V1|" + version + "|" + revision + "|END_MKVTUI_BUILD_V1"
	for _, args := range [][]string{{"--version"}, {"version"}, {"version", "--json"}} {
		r, w, e := os.Pipe()
		if e != nil {
			t.Fatal(e)
		}
		old := os.Stdout
		os.Stdout = w
		os.Args = append([]string{"makemkv-tui"}, args...)
		e = run()
		os.Stdout = old
		w.Close()
		data, re := io.ReadAll(r)
		r.Close()
		if e != nil || re != nil || !strings.Contains(string(data), "v9.8.7") {
			t.Fatal(e, re, string(data))
		}
		if len(args) == 2 && !strings.Contains(string(data), revision) {
			t.Fatal("missing embedded commit")
		}
	}
}
