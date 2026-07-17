package doctor_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/doctor"
	"github.com/Slothtron/mwt/internal/git"
)

type fakeGit struct {
	lists   map[string][]git.Worktree
	listErr map[string]error
}

func (f *fakeGit) List(repoPath string) ([]git.Worktree, error) {
	key := filepath.Base(repoPath)
	if f.listErr != nil {
		if err, ok := f.listErr[key]; ok {
			return nil, err
		}
	}
	if f.lists == nil {
		return nil, nil
	}
	return f.lists[key], nil
}

// memFS is a minimal in-memory FS for doctor tests.
type memFS struct {
	dirs  map[string]bool
	files map[string]bool
}

func newMemFS() *memFS {
	return &memFS{
		dirs:  map[string]bool{},
		files: map[string]bool{},
	}
}

func (m *memFS) addDir(path string) {
	path = filepath.Clean(path)
	for {
		m.dirs[path] = true
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
}

func (m *memFS) addFile(path string) {
	path = filepath.Clean(path)
	m.files[path] = true
	m.addDir(filepath.Dir(path))
}

func (m *memFS) Stat(name string) (fs.FileInfo, error) {
	name = filepath.Clean(name)
	if m.dirs[name] {
		return memInfo{name: filepath.Base(name), dir: true}, nil
	}
	if m.files[name] {
		return memInfo{name: filepath.Base(name), dir: false}, nil
	}
	return nil, os.ErrNotExist
}

func (m *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	name = filepath.Clean(name)
	if !m.dirs[name] {
		return nil, os.ErrNotExist
	}
	seen := map[string]bool{}
	var out []fs.DirEntry
	prefix := name + string(filepath.Separator)
	for d := range m.dirs {
		if !strings.HasPrefix(d, prefix) {
			continue
		}
		rest := strings.TrimPrefix(d, prefix)
		seg, _, _ := strings.Cut(rest, string(filepath.Separator))
		if seg == "" || seen[seg] {
			continue
		}
		// only immediate children
		if strings.Contains(rest, string(filepath.Separator)) {
			// still a child dir if first segment is a dir we know
			child := filepath.Join(name, seg)
			if m.dirs[child] {
				seen[seg] = true
				out = append(out, memInfo{name: seg, dir: true})
			}
			continue
		}
		seen[seg] = true
		out = append(out, memInfo{name: seg, dir: true})
	}
	for f := range m.files {
		if filepath.Dir(f) != name {
			continue
		}
		base := filepath.Base(f)
		if seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, memInfo{name: base, dir: false})
	}
	return out, nil
}

type memInfo struct {
	name string
	dir  bool
}

func (m memInfo) Name() string               { return m.name }
func (m memInfo) IsDir() bool                { return m.dir }
func (m memInfo) Type() fs.FileMode          { return m.Mode().Type() }
func (m memInfo) Info() (fs.FileInfo, error) { return m, nil }
func (m memInfo) Size() int64                { return 0 }
func (m memInfo) Mode() fs.FileMode {
	if m.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (m memInfo) ModTime() time.Time { return time.Time{} }
func (m memInfo) Sys() any           { return nil }

func testCfg(root string, repos ...string) *config.Config {
	return &config.Config{
		Root:         ".",
		WorktreePath: "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}",
		Repos:         repos,
		MetaRoot:     root,
		ConfigPath:   filepath.Join(root, ".mwt.yaml"),
	}
}

func TestCheck_Healthy(t *testing.T) {
	root := "/meta"
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	mfs := newMemFS()
	mfs.addDir(root)
	mfs.addDir(main)
	mfs.addDir(wt)

	g := &fakeGit{lists: map[string][]git.Worktree{
		"sap": {
			{Path: main, Branch: "master"},
			{Path: wt, Branch: "feat"},
		},
	}}
	c := &doctor.Checker{Git: g, FS: mfs}
	findings, err := c.Check(testCfg(root, "sap"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("want healthy, got %#v", findings)
	}
}

func TestCheck_MainMissing(t *testing.T) {
	root := "/meta"
	mfs := newMemFS()
	mfs.addDir(root)

	c := &doctor.Checker{Git: &fakeGit{}, FS: mfs}
	findings, err := c.Check(testCfg(root, "sap"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Kind != doctor.KindMainMissing {
		t.Fatalf("got %#v", findings)
	}
	if !strings.Contains(findings[0].Path, "sap") {
		t.Fatalf("path=%s", findings[0].Path)
	}
}

func TestCheck_PrunableSuggestsCanonicalPath(t *testing.T) {
	root := "/meta"
	main := filepath.Join(root, "sap")
	stale := filepath.Join(root, "oldtrees", "sap", "feat")
	canonical := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	mfs := newMemFS()
	mfs.addDir(root)
	mfs.addDir(main)
	// stale path intentionally absent

	g := &fakeGit{lists: map[string][]git.Worktree{
		"sap": {
			{Path: main, Branch: "master"},
			{Path: stale, Branch: "feat"},
		},
	}}
	c := &doctor.Checker{Git: g, FS: mfs}
	findings, err := c.Check(testCfg(root, "sap"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Kind != doctor.KindPrunable {
		t.Fatalf("got %#v", findings)
	}
	joined := strings.Join(findings[0].Suggest, "\n")
	if !strings.Contains(joined, "git -C "+main+" worktree prune") {
		t.Fatalf("missing prune: %s", joined)
	}
	if !strings.Contains(joined, "mwt add feat --repos sap") {
		t.Fatalf("missing re-add: %s", joined)
	}
	if !strings.Contains(joined, canonical) {
		t.Fatalf("canonical path missing from suggestions: %s", joined)
	}
}

func TestCheck_PrunableUsesDotWorktreesDefault(t *testing.T) {
	root := "/meta"
	main := filepath.Join(root, "sap")
	stale := filepath.Join(root, "elsewhere", "feat")
	canonical := filepath.Join(root, ".worktrees", "sap", "feat", "sap")
	mfs := newMemFS()
	mfs.addDir(root)
	mfs.addDir(main)

	cfg := &config.Config{
		Root:         ".",
		WorktreePath: config.DefaultWorktreePathWithGit, // as Load would set with .git
		Repos:         []string{"sap"},
		MetaRoot:     root,
		HasGitAtRoot: true,
	}
	g := &fakeGit{lists: map[string][]git.Worktree{
		"sap": {
			{Path: main, Branch: "master"},
			{Path: stale, Branch: "feat"},
		},
	}}
	c := &doctor.Checker{Git: g, FS: mfs}
	findings, err := c.Check(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %#v", findings)
	}
	joined := strings.Join(findings[0].Suggest, "\n")
	if !strings.Contains(joined, canonical) {
		t.Fatalf("want .worktrees canonical, got: %s", joined)
	}
}

func TestCheck_Unregistered(t *testing.T) {
	root := "/meta"
	main := filepath.Join(root, "sap")
	orphan := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	mfs := newMemFS()
	mfs.addDir(root)
	mfs.addDir(main)
	mfs.addDir(orphan)

	g := &fakeGit{lists: map[string][]git.Worktree{
		"sap": {{Path: main, Branch: "master"}},
	}}
	c := &doctor.Checker{Git: g, FS: mfs}
	findings, err := c.Check(testCfg(root, "sap"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Kind != doctor.KindUnregistered {
		t.Fatalf("got %#v", findings)
	}
	if findings[0].Branch != "feat" || findings[0].Path != orphan {
		t.Fatalf("finding=%#v", findings[0])
	}
	joined := strings.Join(findings[0].Suggest, "\n")
	if !strings.Contains(joined, "mwt add feat --repos sap") {
		t.Fatalf("suggest=%s", joined)
	}
	if !strings.Contains(joined, "will not auto-delete") {
		t.Fatalf("should warn against auto-delete: %s", joined)
	}
}

func TestCheck_SetupMissingFromCopySteps(t *testing.T) {
	root := "/meta"
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	src := filepath.Join(main, ".env")
	mfs := newMemFS()
	mfs.addDir(root)
	mfs.addDir(main)
	mfs.addDir(wt)
	mfs.addFile(src)
	// dest .env intentionally missing

	cfg := testCfg(root, "sap")
	cfg.Setup = []config.SetupStep{
		{Copy: &config.CopyAction{
			From: "{{MAIN_PATH}}/.env",
			To:   ".env",
		}},
	}
	g := &fakeGit{lists: map[string][]git.Worktree{
		"sap": {
			{Path: main, Branch: "master"},
			{Path: wt, Branch: "feat"},
		},
	}}
	c := &doctor.Checker{Git: g, FS: mfs}
	findings, err := c.Check(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Kind != doctor.KindSetupMissing {
		t.Fatalf("got %#v", findings)
	}
	if findings[0].Path != filepath.Join(wt, ".env") {
		t.Fatalf("path=%s", findings[0].Path)
	}
	if !strings.Contains(strings.Join(findings[0].Suggest, "\n"), "mwt setup feat --repos sap") {
		t.Fatalf("suggest=%v", findings[0].Suggest)
	}
}

func TestCheck_SetupMissingSkippedWhenSrcAbsent(t *testing.T) {
	root := "/meta"
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	mfs := newMemFS()
	mfs.addDir(root)
	mfs.addDir(main)
	mfs.addDir(wt)

	cfg := testCfg(root, "sap")
	cfg.Setup = []config.SetupStep{
		{Copy: &config.CopyAction{
			From: "{{MAIN_PATH}}/.env",
			To:   ".env",
			// skip_if_missing_src default true
		}},
	}
	g := &fakeGit{lists: map[string][]git.Worktree{
		"sap": {
			{Path: main, Branch: "master"},
			{Path: wt, Branch: "feat"},
		},
	}}
	c := &doctor.Checker{Git: g, FS: mfs}
	findings, err := c.Check(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("want no findings when src missing+skip, got %#v", findings)
	}
}

func TestFormatReport_EmptyAndFindings(t *testing.T) {
	var buf bytes.Buffer
	if err := doctor.FormatReport(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ok: no issues found") {
		t.Fatalf("got %q", buf.String())
	}

	buf.Reset()
	err := doctor.FormatReport(&buf, []doctor.Finding{{
		Kind:    doctor.KindPrunable,
		Repo:    "sap",
		Branch:  "feat",
		Message: "registered worktree path missing (prunable): /x",
		Suggest: []string{"git -C /m worktree prune", "mwt add feat --repos sap"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "[prunable] sap:") {
		t.Fatalf("header: %q", out)
	}
	if !strings.Contains(out, "branch: feat") || !strings.Contains(out, "suggest:") {
		t.Fatalf("body: %q", out)
	}
}

func TestCheck_RealFS_UnregisteredAndDualDefault(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "oauth")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, ".worktrees", "oauth", "demo", "oauth")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Root:         ".",
		WorktreePath: config.DefaultWorktreePathWithGit,
		Repos:         []string{"oauth"},
		MetaRoot:     root,
		HasGitAtRoot: true,
	}
	g := &fakeGit{lists: map[string][]git.Worktree{
		"oauth": {{Path: main, Branch: "master"}},
	}}
	c := &doctor.Checker{Git: g, FS: doctor.OSFS{}}
	findings, err := c.Check(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Kind != doctor.KindUnregistered {
		t.Fatalf("got %#v", findings)
	}
	if findings[0].Path != orphan {
		t.Fatalf("path=%s want %s", findings[0].Path, orphan)
	}
}
