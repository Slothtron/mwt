package git_test

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/git"
)

// fakeRunner records git invocations and returns scripted results.
type fakeRunner struct {
	// calls accumulates "arg0 arg1 ..." (without -C / repo).
	calls []string
	// handlers map a joined-args prefix to a response.
	// Matching uses exact joined args first, then longest prefix.
	exact map[string]fakeResult
	// showRefExists controls show-ref --verify for BranchExists.
	// Key is full ref (refs/heads/X); missing key → exit 1 (not exists).
	showRefExists map[string]bool
	// defaultFail makes unmatched commands fail.
	defaultFail bool
}

type fakeResult struct {
	stdout string
	err    error
}

func (f *fakeRunner) Git(repoPath string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	f.calls = append(f.calls, joined)

	// BranchExists path: show-ref --verify --quiet refs/heads/...
	if len(args) >= 4 && args[0] == "show-ref" && args[1] == "--verify" && args[2] == "--quiet" {
		ref := args[3]
		if f.showRefExists != nil && f.showRefExists[ref] {
			return "", nil
		}
		return "", &git.CommandError{
			RepoPath: repoPath,
			Args:     args,
			Stderr:   "missing",
			Err:      exitErr(1),
		}
	}

	if f.exact != nil {
		if res, ok := f.exact[joined]; ok {
			if res.err != nil {
				return res.stdout, wrap(repoPath, args, res.err, res.stdout)
			}
			return res.stdout, nil
		}
	}

	if f.defaultFail {
		return "", &git.CommandError{
			RepoPath: repoPath,
			Args:     args,
			Stderr:   "unexpected command: " + joined,
			Err:      exitErr(1),
		}
	}
	return "", nil
}

func wrap(repo string, args []string, err error, stdout string) error {
	if ce, ok := err.(*git.CommandError); ok {
		ce.RepoPath = repo
		ce.Args = args
		return ce
	}
	return &git.CommandError{
		RepoPath: repo,
		Args:     args,
		Stderr:   err.Error(),
		Err:      err,
	}
}

func exitErr(code int) error {
	// Build a real *exec.ExitError so ExitCode() works.
	cmd := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code))
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee
	}
	return err
}

func fail(stderr string) error {
	return &git.CommandError{
		Stderr: stderr,
		Err:    exitErr(128),
	}
}

func TestAdd_SuccessExistingBranch(t *testing.T) {
	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree add /wt/feat feat": {},
		},
		defaultFail: true,
	}
	a := &git.Adapter{Runner: f}

	if err := a.Add("/repos/oauth", "/wt/feat", "feat", ""); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "worktree add /wt/feat feat" {
		t.Fatalf("calls = %#v", f.calls)
	}
}

func TestAdd_MissingBranch_WithFrom_CreatesBranch(t *testing.T) {
	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree add /wt/feat feat": {
				err: fail("fatal: invalid reference: feat"),
			},
			"worktree add -b feat /wt/feat main": {},
		},
		showRefExists: map[string]bool{}, // feat missing
		defaultFail:   true,
	}
	a := &git.Adapter{Runner: f}

	if err := a.Add("/repos/oauth", "/wt/feat", "feat", "main"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	want := []string{
		"worktree add /wt/feat feat",
		"show-ref --verify --quiet refs/heads/feat",
		"worktree add -b feat /wt/feat main",
	}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls =\n  %v\nwant\n  %v", f.calls, want)
	}
}

func TestAdd_MissingBranch_WithoutFrom_SurfacesError(t *testing.T) {
	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree add /wt/feat feat": {
				err: fail("fatal: invalid reference: feat"),
			},
		},
		defaultFail: true,
	}
	a := &git.Adapter{Runner: f}

	err := a.Add("/repos/oauth", "/wt/feat", "feat", "")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "/repos/oauth") {
		t.Fatalf("error should mention repo path, got: %v", err)
	}
	if !strings.Contains(msg, "invalid reference") {
		t.Fatalf("error should mention git stderr, got: %v", err)
	}
	// Must not attempt -b
	for _, c := range f.calls {
		if strings.Contains(c, "-b") {
			t.Fatalf("unexpected -b call: %v", f.calls)
		}
	}
}

func TestAdd_BranchExists_OtherFailure_NoRetry(t *testing.T) {
	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree add /wt/feat feat": {
				err: fail("fatal: '/wt/feat' already exists"),
			},
		},
		showRefExists: map[string]bool{"refs/heads/feat": true},
		defaultFail:   true,
	}
	a := &git.Adapter{Runner: f}

	err := a.Add("/repos/oauth", "/wt/feat", "feat", "main")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("got: %v", err)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "-b") {
			t.Fatalf("should not retry with -b when branch exists: %v", f.calls)
		}
	}
}

func TestAdd_CreateBranchFailure_SurfacesRepo(t *testing.T) {
	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree add /wt/feat feat": {
				err: fail("fatal: invalid reference: feat"),
			},
			"worktree add -b feat /wt/feat main": {
				err: fail("fatal: invalid reference: main"),
			},
		},
		showRefExists: map[string]bool{},
		defaultFail:   true,
	}
	a := &git.Adapter{Runner: f}

	err := a.Add("/repos/sap", "/wt/feat", "feat", "main")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "/repos/sap") {
		t.Fatalf("error should mention repo, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid reference: main") {
		t.Fatalf("error should mention from-point failure, got: %v", err)
	}
}

func TestRemove_Force(t *testing.T) {
	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree remove --force /wt/feat": {},
		},
		defaultFail: true,
	}
	a := &git.Adapter{Runner: f}
	if err := a.Remove("/repos/oauth", "/wt/feat", true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestRemove_Failure(t *testing.T) {
	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree remove /wt/feat": {
				err: fail("fatal: '/wt/feat' contains modified or untracked files"),
			},
		},
		defaultFail: true,
	}
	a := &git.Adapter{Runner: f}
	err := a.Remove("/repos/oauth", "/wt/feat", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "/repos/oauth") {
		t.Fatalf("got: %v", err)
	}
}

func TestList_Porcelain(t *testing.T) {
	porcelain := "" +
		"worktree /repos/oauth\n" +
		"HEAD abc123\n" +
		"branch refs/heads/main\n" +
		"\n" +
		"worktree /wt/feat\n" +
		"HEAD def456\n" +
		"branch refs/heads/feat\n" +
		"\n" +
		"worktree /wt/detached\n" +
		"HEAD ghi789\n" +
		"detached\n" +
		"\n"

	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree list --porcelain": {stdout: porcelain},
		},
		defaultFail: true,
	}
	a := &git.Adapter{Runner: f}

	got, err := a.List("/repos/oauth")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %#v", len(got), got)
	}
	if got[0].Path != "/repos/oauth" || got[0].Branch != "main" || got[0].HEAD != "abc123" {
		t.Fatalf("entry0 = %#v", got[0])
	}
	if got[1].Path != "/wt/feat" || got[1].Branch != "feat" {
		t.Fatalf("entry1 = %#v", got[1])
	}
	if !got[2].Detached || got[2].Branch != "" {
		t.Fatalf("entry2 = %#v", got[2])
	}
}

func TestList_Failure(t *testing.T) {
	f := &fakeRunner{
		exact: map[string]fakeResult{
			"worktree list --porcelain": {
				err: fail("fatal: not a git repository"),
			},
		},
		defaultFail: true,
	}
	a := &git.Adapter{Runner: f}
	_, err := a.List("/missing/repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "/missing/repo") {
		t.Fatalf("got: %v", err)
	}
}

func TestCommandError_IncludesRepoAndStderr(t *testing.T) {
	err := &git.CommandError{
		RepoPath: "/repos/oauth",
		Args:     []string{"worktree", "add", "/wt", "feat"},
		Stderr:   "fatal: invalid reference: feat",
		Err:      exitErr(128),
	}
	msg := err.Error()
	for _, want := range []string{"/repos/oauth", "worktree add", "invalid reference"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Error() = %q, want substring %q", msg, want)
		}
	}
}
