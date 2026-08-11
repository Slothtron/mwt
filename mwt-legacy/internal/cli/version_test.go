package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/cli"
)

func TestVersion_Command(t *testing.T) {
	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "mwt version ") {
		t.Fatalf("got %q", out.String())
	}
}

func TestVersion_RootFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "mwt version ") {
		t.Fatalf("got %q", out.String())
	}
}
