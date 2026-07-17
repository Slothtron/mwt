package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/cli"
)

func TestSkillSync_Dir(t *testing.T) {
	parent := t.TempDir()
	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"skill", "sync", "--dir", parent})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(parent, "mwt", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: mwt") {
		t.Fatalf("bad skill content")
	}
	if !strings.Contains(out.String(), "synced skill to") {
		t.Fatalf("output: %s", out.String())
	}
}

func TestSkill_BareDefaultsToSync(t *testing.T) {
	parent := t.TempDir()
	cmd := cli.NewRootCmd()
	cmd.SetArgs([]string{"skill", "--dir", parent})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, "mwt", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSkillSync_Force(t *testing.T) {
	parent := t.TempDir()
	run := func(force bool) error {
		cmd := cli.NewRootCmd()
		args := []string{"skill", "sync", "--dir", parent}
		if force {
			args = append(args, "--force")
		}
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run(false); err != nil {
		t.Fatal(err)
	}
	if err := run(false); err == nil {
		t.Fatal("expected error without --force")
	}
	if err := run(true); err != nil {
		t.Fatal(err)
	}
}
