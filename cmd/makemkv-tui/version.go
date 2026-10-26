package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
)

// Linker variables are supplied by tools/release. Module installs also have a
// trusted module version available in Go build metadata; no future tag is hardcoded.
var revision = "unknown"
var releaseIdentity string

func resolveVersion(tag, commit, identity string, info *debug.BuildInfo) (string, string) {
	// One linked string drives both CLI identity and cross-platform validation.
	// Go intentionally omits -ldflags from BuildInfo when -trimpath is enabled.
	if parts := strings.Split(identity, "|"); len(parts) == 4 && parts[0] == "MKVTUI_BUILD_V1" && parts[3] == "END_MKVTUI_BUILD_V1" {
		tag, commit = parts[1], parts[2]
	}
	if info != nil {
		if tag == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			tag = info.Main.Version
		}
		if commit == "unknown" {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					commit = s.Value
				}
			}
		}
	}
	return tag, commit
}

func versionInfo() map[string]string {
	info, _ := debug.ReadBuildInfo()
	tag, commit := resolveVersion(version, revision, releaseIdentity, info)
	return map[string]string{"version": tag, "commit": commit, "go_version": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH}
}
func printVersion(asJSON bool) error {
	info := versionInfo()
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(info)
	}
	fmt.Println("makemkv-tui " + info["version"])
	return nil
}
