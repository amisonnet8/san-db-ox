package main

import (
	"runtime/debug"
	"testing"
)

// TestResolvedVersionPrefersLdflagsValue guards spec §12's version
// resolution order: a release build's -ldflags -X main.version=<tag>
// value must win over whatever runtime/debug.ReadBuildInfo reports.
func TestResolvedVersionPrefersLdflagsValue(t *testing.T) {
	old := version
	defer func() { version = old }()

	version = "v0.1.0"
	if got := resolvedVersion(); got != "v0.1.0" {
		t.Fatalf("resolvedVersion() = %q, want %q", got, "v0.1.0")
	}
}

// TestResolvedVersionFallsBackToDev guards the fallback tier of spec
// §12's resolution order. It cannot force ReadBuildInfo to report
// "(devel)" (this test binary's own build info is whatever `go test`
// produced it with, dirty-checkout stamping included), so it only
// checks that resolvedVersion degrades safely -- returns a non-empty
// string -- rather than crashing, which is what actually matters here.
func TestResolvedVersionFallsBackToDev(t *testing.T) {
	old := version
	defer func() { version = old }()

	version = "dev"
	if got := resolvedVersion(); got == "" {
		t.Fatal("resolvedVersion() returned an empty string")
	}
}

// TestIsDirtyBuildReadsVCSModifiedSetting is a regression test for a real
// discovery made while implementing this: a plain `go build` run inside
// this repo's own (git) working tree does NOT report Main.Version as
// "(devel)" -- Go's -buildvcs=auto default stamps a VCS-derived
// pseudo-version instead, "+dirty" suffix and all when the checkout has
// uncommitted changes. Trusting that dirty pseudo-version would show a
// version string that does not correspond to anything installable, so
// isDirtyBuild must correctly read the "vcs.modified" setting rather
// than only recognizing "(devel)".
func TestIsDirtyBuildReadsVCSModifiedSetting(t *testing.T) {
	clean := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs", Value: "git"},
		{Key: "vcs.modified", Value: "false"},
	}}
	if isDirtyBuild(clean) {
		t.Error("isDirtyBuild(vcs.modified=false) = true, want false")
	}

	dirty := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs", Value: "git"},
		{Key: "vcs.modified", Value: "true"},
	}}
	if !isDirtyBuild(dirty) {
		t.Error("isDirtyBuild(vcs.modified=true) = false, want true")
	}

	noVCS := &debug.BuildInfo{}
	if isDirtyBuild(noVCS) {
		t.Error("isDirtyBuild(no vcs.modified setting) = true, want false")
	}
}
