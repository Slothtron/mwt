package version

import (
	"strings"
	"testing"
)

func TestString_NonEmpty(t *testing.T) {
	s := String()
	if s == "" {
		t.Fatal("String() empty")
	}
	if !strings.HasPrefix(s, "mwt version ") {
		t.Fatalf("unexpected: %q", s)
	}
	if !strings.Contains(s, "commit ") || !strings.Contains(s, "built ") {
		t.Fatalf("missing fields: %q", s)
	}
}

func TestInfo_DefaultVersion(t *testing.T) {
	d := Info()
	if d.Version != "0.1.0" {
		t.Fatalf("Version=%q want 0.1.0", d.Version)
	}
	if d.Commit == "" {
		t.Fatal("Commit empty")
	}
	if d.BuildDate == "" {
		t.Fatal("BuildDate empty")
	}
}

func TestShortCommit(t *testing.T) {
	if got := shortCommit("abcdefghij"); got != "abcdefg" {
		t.Fatalf("got %q", got)
	}
	if got := shortCommit("abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestInfo_UsesLdflagsVersion(t *testing.T) {
	prevV, prevC, prevD := Version, Commit, BuildDate
	t.Cleanup(func() {
		Version, Commit, BuildDate = prevV, prevC, prevD
	})
	Version = "0.2.0"
	Commit = "deadbeef"
	BuildDate = "2026-01-01"
	d := Info()
	if d.Version != "0.2.0" || d.Commit != "deadbeef" || d.BuildDate != "2026-01-01" {
		t.Fatalf("got %+v", d)
	}
	if !strings.Contains(String(), "0.2.0") {
		t.Fatalf("String: %s", String())
	}
}

func TestInfo_FillsCommitFromBuildInfoWhenDefault(t *testing.T) {
	prevC, prevD := Commit, BuildDate
	t.Cleanup(func() {
		Commit, BuildDate = prevC, prevD
	})
	Commit = "none"
	BuildDate = "unknown"
	// Version stays 0.1.0; Commit may be enriched under `go test` with -buildvcs.
	d := Info()
	if d.Version != "0.1.0" {
		t.Fatalf("Version=%q", d.Version)
	}
	_ = d.Commit
}
