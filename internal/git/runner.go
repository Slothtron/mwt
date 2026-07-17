package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes git commands against a main-checkout directory.
type Runner interface {
	// Git runs: git -C repoPath <args...>
	// On non-zero exit, returns a *CommandError with stderr.
	Git(repoPath string, args ...string) (stdout string, err error)
}

// ExecRunner invokes the system git binary via os/exec.
type ExecRunner struct {
	// Bin is the git executable; empty means "git".
	Bin string
}

// Git implements Runner.
func (r ExecRunner) Git(repoPath string, args ...string) (string, error) {
	bin := r.Bin
	if bin == "" {
		bin = "git"
	}
	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command(bin, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String()
	if err != nil {
		return out, &CommandError{
			RepoPath: repoPath,
			Args:     args,
			Stderr:   strings.TrimSpace(stderr.String()),
			Err:      err,
		}
	}
	return out, nil
}

// CommandError is a failed git invocation, scoped to one repo path.
type CommandError struct {
	RepoPath string
	Args     []string
	Stderr   string
	Err      error
}

func (e *CommandError) Error() string {
	msg := e.Stderr
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	if msg == "" {
		msg = "command failed"
	}
	return fmt.Sprintf("git %s (repo %s): %s", strings.Join(e.Args, " "), e.RepoPath, msg)
}

func (e *CommandError) Unwrap() error { return e.Err }
