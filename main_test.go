package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/looztra/gh-sync/internal/git"
)

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		verbose   bool
		dryRun    bool
		wantErr   bool
		isHelp    bool
		isVersion bool
	}{
		{"no flags", nil, false, false, false, false, false},
		{"verbose long", []string{"--verbose"}, true, false, false, false, false},
		{"verbose short", []string{"-v"}, true, false, false, false, false},
		{"dry-run long", []string{"--dry-run"}, false, true, false, false, false},
		{"dry-run short", []string{"-n"}, false, true, false, false, false},
		{"verbose and dry-run", []string{"-v", "-n"}, true, true, false, false, false},
		{"unknown flag", []string{"--unknown"}, false, false, true, false, false},
		{"help long", []string{"--help"}, false, false, true, true, false},
		{"help short", []string{"-h"}, false, false, true, true, false},
		{"version long", []string{"--version"}, false, false, true, false, true},
		{"version short", []string{"-V"}, false, false, true, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseArgs(tt.args)
			if tt.isHelp {
				if !errors.Is(err, errHelp) {
					t.Fatalf("parseArgs(%v) error = %v, want errHelp", tt.args, err)
				}
				return
			}
			if tt.isVersion {
				if !errors.Is(err, errVersion) {
					t.Fatalf("parseArgs(%v) error = %v, want errVersion", tt.args, err)
				}
				return
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if opts.verbose != tt.verbose {
				t.Errorf("verbose = %v, want %v", opts.verbose, tt.verbose)
			}
			if opts.dryRun != tt.dryRun {
				t.Errorf("dryRun = %v, want %v", opts.dryRun, tt.dryRun)
			}
		})
	}
}

func TestPrintUsage(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)

	output := buf.String()
	for _, want := range []string{"--verbose", "--dry-run", "--version, -V", "--help", "gh sync"} {
		if !strings.Contains(output, want) {
			t.Errorf("usage output missing %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

type testEnv struct {
	base   string
	remote string
	local  string
	t      *testing.T
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	local := filepath.Join(base, "local")
	setup := filepath.Join(base, "setup")

	mustExec(t, "", "git", "init", "-b", "main", setup)
	mustExec(t, setup, "git", "config", "user.email", "test@test.com")
	mustExec(t, setup, "git", "config", "user.name", "Test")
	writeTestFile(t, filepath.Join(setup, "README.md"), "# test\n")
	mustExec(t, setup, "git", "add", ".")
	mustExec(t, setup, "git", "commit", "-m", "initial")
	mustExec(t, "", "git", "clone", "--bare", setup, remote)
	mustExec(t, "", "git", "clone", remote, local)
	mustExec(t, local, "git", "config", "user.email", "test@test.com")
	mustExec(t, local, "git", "config", "user.name", "Test")

	return &testEnv{base: base, remote: remote, local: local, t: t}
}

// chdir changes into the local repo and restores the original dir on cleanup.
func (e *testEnv) chdir() {
	e.t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		e.t.Fatal(err)
	}
	if err := os.Chdir(e.local); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { os.Chdir(orig) })

	git.Verbose = false
	git.Color = false
	git.Stderr = os.Stderr
}

var tmpCounter int

// addRemoteCommit pushes a new commit to the given branch on the bare remote.
func (e *testEnv) addRemoteCommit(branch, filename, content string) {
	e.t.Helper()
	tmpCounter++
	tmp := filepath.Join(e.base, fmt.Sprintf("tmp-push-%s-%d", branch, tmpCounter))
	mustExec(e.t, "", "git", "clone", e.remote, tmp)
	mustExec(e.t, tmp, "git", "config", "user.email", "test@test.com")
	mustExec(e.t, tmp, "git", "config", "user.name", "Test")
	mustExec(e.t, tmp, "git", "checkout", branch)
	writeTestFile(e.t, filepath.Join(tmp, filename), content)
	mustExec(e.t, tmp, "git", "add", ".")
	mustExec(e.t, tmp, "git", "commit", "-m", "remote update: "+filename)
	mustExec(e.t, tmp, "git", "push")
	os.RemoveAll(tmp)
}

// createBranch creates a branch locally with one commit and pushes it with tracking.
func (e *testEnv) createBranch(name, filename, content string) {
	e.t.Helper()
	mustExec(e.t, e.local, "git", "checkout", "-b", name)
	writeTestFile(e.t, filepath.Join(e.local, filename), content)
	mustExec(e.t, e.local, "git", "add", ".")
	mustExec(e.t, e.local, "git", "commit", "-m", "add "+filename)
	mustExec(e.t, e.local, "git", "push", "-u", "origin", name)
	mustExec(e.t, e.local, "git", "checkout", "main")
}

// deleteRemoteBranch removes a branch from the bare remote.
func (e *testEnv) deleteRemoteBranch(name string) {
	e.t.Helper()
	mustExec(e.t, e.local, "git", "push", "origin", "--delete", name)
}

// mergeOnRemote merges a branch into main on the remote (regular merge).
func (e *testEnv) mergeOnRemote(branch string) {
	e.t.Helper()
	tmp := filepath.Join(e.base, "tmp-merge-"+branch)
	mustExec(e.t, "", "git", "clone", e.remote, tmp)
	mustExec(e.t, tmp, "git", "config", "user.email", "test@test.com")
	mustExec(e.t, tmp, "git", "config", "user.name", "Test")
	mustExec(e.t, tmp, "git", "merge", "origin/"+branch)
	mustExec(e.t, tmp, "git", "push")
	os.RemoveAll(tmp)
}

// squashMergeOnRemote squash-merges a branch into main on the remote.
func (e *testEnv) squashMergeOnRemote(branch string) {
	e.t.Helper()
	tmp := filepath.Join(e.base, "tmp-squash-"+branch)
	mustExec(e.t, "", "git", "clone", e.remote, tmp)
	mustExec(e.t, tmp, "git", "config", "user.email", "test@test.com")
	mustExec(e.t, tmp, "git", "config", "user.name", "Test")
	mustExec(e.t, tmp, "git", "merge", "--squash", "origin/"+branch)
	mustExec(e.t, tmp, "git", "commit", "-m", "squash merge "+branch)
	mustExec(e.t, tmp, "git", "push")
	os.RemoveAll(tmp)
}

func runSync(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	err = sync(&outBuf, &errBuf, false, false)
	return outBuf.String(), errBuf.String(), err
}

func runSyncDry(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	err = sync(&outBuf, &errBuf, false, true)
	return outBuf.String(), errBuf.String(), err
}

// ---------------------------------------------------------------------------

func TestSync_UpToDate(t *testing.T) {
	env := newTestEnv(t)
	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Errorf("expected no stdout for up-to-date repo, got: %s", stdout)
	}
}

func TestSync_FastForwardCurrentBranch(t *testing.T) {
	env := newTestEnv(t)
	env.addRemoteCommit("main", "new.txt", "new content\n")
	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Updated branch main") {
		t.Errorf("expected update message for main, got stdout: %s", stdout)
	}
}

func TestSync_FastForwardOtherBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("feature", "feature.txt", "v1\n")
	env.addRemoteCommit("feature", "feature.txt", "v2\n")
	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Updated branch feature") {
		t.Errorf("expected update message for feature, got stdout: %s", stdout)
	}
}

func TestSync_DivergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("diverged", "file.txt", "original\n")

	// Add a remote commit on the branch
	env.addRemoteCommit("diverged", "remote.txt", "from remote\n")

	// Add a local commit on the branch
	mustExec(t, env.local, "git", "checkout", "diverged")
	writeTestFile(t, filepath.Join(env.local, "local.txt"), "local change\n")
	mustExec(t, env.local, "git", "add", ".")
	mustExec(t, env.local, "git", "commit", "-m", "local change")
	mustExec(t, env.local, "git", "checkout", "main")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstdout: %s", err, stdout)
	}
	if !strings.Contains(stderr, "unpushed commits") {
		t.Errorf("expected unpushed warning, got stderr: %s", stderr)
	}
}

func TestSync_DeleteMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("merged-feature", "merged.txt", "content\n")

	env.mergeOnRemote("merged-feature")
	env.deleteRemoteBranch("merged-feature")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Deleted branch merged-feature") {
		t.Errorf("expected delete message, got stdout: %s", stdout)
	}

	// Verify the branch is actually gone
	out := mustExec(t, env.local, "git", "branch", "--list", "merged-feature")
	if strings.TrimSpace(out) != "" {
		t.Error("branch merged-feature should have been deleted")
	}
}

func TestSync_DeleteSquashMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("squash-me", "squash.txt", "squash content\n")

	env.squashMergeOnRemote("squash-me")
	env.deleteRemoteBranch("squash-me")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Deleted branch squash-me") {
		t.Errorf("expected delete message for squash-merged branch, got stdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestSync_NotMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("not-merged", "unique.txt", "unique\n")

	// Delete from remote without merging
	env.deleteRemoteBranch("not-merged")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstdout: %s", err, stdout)
	}
	if !strings.Contains(stderr, "not merged") {
		t.Errorf("expected not-merged warning, got stderr: %s", stderr)
	}
}

func TestSync_DeleteCurrentBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("current-gone", "gone.txt", "gone\n")

	env.mergeOnRemote("current-gone")
	env.deleteRemoteBranch("current-gone")

	// Switch to the branch that will be deleted
	mustExec(t, env.local, "git", "checkout", "current-gone")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Deleted branch current-gone") {
		t.Errorf("expected delete message, got stdout: %s", stdout)
	}

	// Verify we switched to the default branch
	branch, err := git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected to be on main after deletion, got %s", branch)
	}
}

func TestSync_DryRunFastForwardCurrentBranch(t *testing.T) {
	env := newTestEnv(t)
	env.addRemoteCommit("main", "new.txt", "new content\n")
	head := strings.TrimSpace(mustExec(t, env.local, "git", "rev-parse", "HEAD"))
	env.chdir()

	stdout, stderr, err := runSyncDry(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Would update branch main") {
		t.Errorf("expected dry-run update message, got stdout: %s", stdout)
	}
	if got := strings.TrimSpace(mustExec(t, env.local, "git", "rev-parse", "HEAD")); got != head {
		t.Errorf("HEAD moved in dry-run: %s -> %s", head, got)
	}
}

func TestSync_DryRunFastForwardOtherBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("feature", "feature.txt", "v1\n")
	env.addRemoteCommit("feature", "feature.txt", "v2\n")
	before := strings.TrimSpace(mustExec(t, env.local, "git", "rev-parse", "refs/heads/feature"))
	env.chdir()

	stdout, stderr, err := runSyncDry(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Would update branch feature") {
		t.Errorf("expected dry-run update message, got stdout: %s", stdout)
	}
	if strings.Contains(stdout, "Updated branch") {
		t.Errorf("dry-run must not report an update, got stdout: %s", stdout)
	}
	if got := strings.TrimSpace(mustExec(t, env.local, "git", "rev-parse", "refs/heads/feature")); got != before {
		t.Errorf("feature ref moved in dry-run: %s -> %s", before, got)
	}
}

func TestSync_DryRunDeleteMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("merged-feature", "merged.txt", "content\n")
	env.mergeOnRemote("merged-feature")
	env.deleteRemoteBranch("merged-feature")
	env.chdir()

	stdout, stderr, err := runSyncDry(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Would delete branch merged-feature") {
		t.Errorf("expected dry-run delete message, got stdout: %s", stdout)
	}
	if !strings.Contains(stdout, "merged into main") {
		t.Errorf("expected reason in stdout, got: %s", stdout)
	}
	if strings.TrimSpace(mustExec(t, env.local, "git", "branch", "--list", "merged-feature")) == "" {
		t.Error("branch merged-feature must survive a dry-run")
	}
}

func TestSync_DryRunDeleteSquashMergedBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("squash-me", "squash.txt", "squash content\n")
	env.squashMergeOnRemote("squash-me")
	env.deleteRemoteBranch("squash-me")
	env.chdir()

	stdout, stderr, err := runSyncDry(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Would delete branch squash-me") || !strings.Contains(stdout, "squash-merged into main") {
		t.Errorf("expected squash-merged dry-run delete message, got stdout: %s\nstderr: %s", stdout, stderr)
	}
	if strings.TrimSpace(mustExec(t, env.local, "git", "branch", "--list", "squash-me")) == "" {
		t.Error("branch squash-me must survive a dry-run")
	}
}

func TestSync_DryRunDeleteCurrentBranch(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("current-gone", "gone.txt", "gone\n")
	env.mergeOnRemote("current-gone")
	env.deleteRemoteBranch("current-gone")
	mustExec(t, env.local, "git", "checkout", "current-gone")
	env.chdir()

	stdout, stderr, err := runSyncDry(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Would switch to main, then delete branch current-gone") {
		t.Errorf("expected dry-run switch+delete message, got stdout: %s", stdout)
	}
	branch, err := git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if branch != "current-gone" {
		t.Errorf("expected to stay on current-gone, got %s", branch)
	}
	if strings.TrimSpace(mustExec(t, env.local, "git", "branch", "--list", "current-gone")) == "" {
		t.Error("branch current-gone must survive a dry-run")
	}
}

func TestSync_DryRunWarningsUnchanged(t *testing.T) {
	env := newTestEnv(t)

	env.createBranch("not-merged", "unique.txt", "unique\n")
	env.deleteRemoteBranch("not-merged")

	env.createBranch("diverged", "file.txt", "original\n")
	env.addRemoteCommit("diverged", "remote.txt", "from remote\n")
	mustExec(t, env.local, "git", "checkout", "diverged")
	writeTestFile(t, filepath.Join(env.local, "local.txt"), "local change\n")
	mustExec(t, env.local, "git", "add", ".")
	mustExec(t, env.local, "git", "commit", "-m", "local change")
	mustExec(t, env.local, "git", "checkout", "main")

	env.chdir()

	stdout, stderr, err := runSyncDry(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstdout: %s", err, stdout)
	}
	if !strings.Contains(stderr, "seems to contain unpushed commits") {
		t.Errorf("expected unpushed warning, got stderr: %s", stderr)
	}
	if !strings.Contains(stderr, "was deleted on origin, but appears not merged into 'main'") {
		t.Errorf("expected not-merged warning, got stderr: %s", stderr)
	}
	if strings.Contains(stdout, "Would delete") {
		t.Errorf("unmerged branch must not be previewed for deletion, got stdout: %s", stdout)
	}
}

func TestSync_ImplicitUpstream(t *testing.T) {
	env := newTestEnv(t)

	// Create a branch on the remote only
	env.addRemoteCommit("main", "setup.txt", "setup\n")

	// Create a local branch with no tracking config
	mustExec(t, env.local, "git", "checkout", "-b", "implicit")
	writeTestFile(t, filepath.Join(env.local, "implicit.txt"), "local\n")
	mustExec(t, env.local, "git", "add", ".")
	mustExec(t, env.local, "git", "commit", "-m", "local implicit")
	mustExec(t, env.local, "git", "push", "origin", "implicit") // push without -u (no tracking)
	mustExec(t, env.local, "git", "checkout", "main")

	// Advance the branch on the remote
	env.addRemoteCommit("implicit", "implicit.txt", "updated\n")

	env.chdir()

	stdout, _, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if !strings.Contains(stdout, "Updated branch implicit") {
		t.Errorf("expected update message for implicit tracking, got stdout: %s", stdout)
	}
}

func TestSync_SkipsBranchCheckedOutInWorktree(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("feature", "feature.txt", "v1\n")
	env.mergeOnRemote("feature")
	env.deleteRemoteBranch("feature")

	wt := filepath.Join(env.base, "wt")
	mustExec(t, env.local, "git", "worktree", "add", wt, "feature")

	env.chdir()

	stdout, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "skipped feature: checked out in worktree") {
		t.Errorf("expected skip message, got stderr: %s", stderr)
	}
	if !strings.Contains(stderr, wt) {
		t.Errorf("expected worktree path %s in stderr: %s", wt, stderr)
	}
	if strings.Contains(stdout, "skipped") {
		t.Errorf("skip message should not go to stdout: %s", stdout)
	}
	out := mustExec(t, env.local, "git", "branch", "--list", "feature")
	if strings.TrimSpace(out) == "" {
		t.Error("branch feature should not have been deleted")
	}
}

func TestSync_DryRunSkipsBranchCheckedOutInWorktree(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("feature", "feature.txt", "v1\n")
	env.mergeOnRemote("feature")
	env.deleteRemoteBranch("feature")

	wt := filepath.Join(env.base, "wt")
	mustExec(t, env.local, "git", "worktree", "add", wt, "feature")

	env.chdir()

	stdout, stderr, err := runSyncDry(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "skipped feature: checked out in worktree "+wt) {
		t.Errorf("expected skip message, got stderr: %s", stderr)
	}
	if strings.Contains(stdout, "Would delete") {
		t.Errorf("worktree branch must not be previewed for deletion, got stdout: %s", stdout)
	}
	if strings.TrimSpace(mustExec(t, env.local, "git", "branch", "--list", "feature")) == "" {
		t.Error("branch feature must survive a dry-run")
	}
}

func TestSync_UnmergedBranchInWorktreeMentionsWorktree(t *testing.T) {
	env := newTestEnv(t)
	env.createBranch("wip", "wip.txt", "v1\n")
	env.deleteRemoteBranch("wip")

	wt := filepath.Join(env.base, "wt")
	mustExec(t, env.local, "git", "worktree", "add", wt, "wip")

	env.chdir()

	_, stderr, err := runSync(t)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "'wip' was deleted on origin, but appears not merged") {
		t.Errorf("expected not-merged warning, got stderr: %s", stderr)
	}
	if !strings.Contains(stderr, "'wip' is checked out in worktree "+wt) {
		t.Errorf("expected worktree note, got stderr: %s", stderr)
	}
}

func TestSync_ColorOutput(t *testing.T) {
	env := newTestEnv(t)
	env.addRemoteCommit("main", "new.txt", "content\n")
	env.chdir()

	var stdout, stderr bytes.Buffer
	err := sync(&stdout, &stderr, true, false)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	if !strings.Contains(stdout.String(), "\033[") {
		t.Errorf("expected ANSI color codes in output, got: %q", stdout.String())
	}
}

func TestSync_VerboseOutput(t *testing.T) {
	env := newTestEnv(t)
	env.chdir()

	var stdout, stderr bytes.Buffer
	git.Verbose = true
	git.Color = false
	git.Stderr = &stderr

	err := sync(&stdout, &stderr, false, false)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	if !strings.Contains(stderr.String(), "$ git") {
		t.Errorf("expected verbose git commands on stderr, got: %q", stderr.String())
	}
}

func TestSync_NoRemotes(t *testing.T) {
	dir := t.TempDir()
	mustExec(t, "", "git", "init", "-b", "main", dir)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	git.Verbose = false
	git.Color = false

	var stdout, stderr bytes.Buffer
	err := sync(&stdout, &stderr, false, false)
	if err == nil {
		t.Fatal("expected error when no remotes exist")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustExec(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v (in %s) failed: %v\n%s", name, args, dir, err, out)
	}
	return string(out)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVersionBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "gh-sync")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin,
		"-ldflags", "-X github.com/looztra/gh-sync/internal/version.Tag=v9.9.9", ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// Run from a directory that is not a git repository.
	want := regexp.MustCompile(`^gh-sync v9\.9\.9 \((?:[0-9a-f]{7}(?:-dirty)?|unknown)\) (?:\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z|unknown)\n$`)
	for _, flag := range []string{"--version", "-V"} {
		cmd := exec.CommandContext(t.Context(), bin, flag)
		cmd.Dir = t.TempDir()
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\nstderr: %s", flag, err, stderr.String())
		}
		if !want.MatchString(stdout.String()) {
			t.Errorf("%s stdout = %q, want match %s", flag, stdout.String(), want)
		}
		if stderr.Len() != 0 {
			t.Errorf("%s stderr = %q, want empty", flag, stderr.String())
		}
	}
}
