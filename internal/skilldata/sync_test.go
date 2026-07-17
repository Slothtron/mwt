package skilldata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSync_WritesSkill(t *testing.T) {
	parent := t.TempDir()
	dest, err := Sync(parent, false)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(parent, "mwt", "SKILL.md")
	if dest != filepath.Join(parent, "mwt") {
		t.Fatalf("dest=%q", dest)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty SKILL.md")
	}
	if !strings.Contains(string(data), "name: mwt") {
		t.Fatalf("missing frontmatter name")
	}
}

func TestSync_RefuseWithoutForce(t *testing.T) {
	parent := t.TempDir()
	if _, err := Sync(parent, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(parent, false); err == nil {
		t.Fatal("expected error when dest exists")
	}
	if _, err := Sync(parent, true); err != nil {
		t.Fatal(err)
	}
}
