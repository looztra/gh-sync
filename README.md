## gh-sync

A [`gh`](https://cli.github.com/) CLI extension that keeps local branches in sync with a remote. It fetches, fast-forwards outdated branches, warns about unpushed work, and cleans up branches whose upstream was deleted after merging (including squash merges).

This was built to replace the [`sync`](https://hub.github.com/hub-sync.1.html) command from the deprecated [`hub`](https://hub.github.com/) tool.

> **Note:** this repository is a fork of [`wassimk/gh-sync`](https://github.com/wassimk/gh-sync). The upstream project has stalled, so we decided to move on with this repo instead. Install and upgrade from `looztra/gh-sync`, not from the original.

### 🛠️ Installation

```shell
gh extension install looztra/gh-sync
```

### 💻 Usage

Run inside any git repository:

```shell
gh sync
```

That's it. The command will:

1. Find the primary remote (`upstream` > `github` > `origin`)
2. Fetch with pruning
3. For each local branch:
   - **Fast-forward** if the branch is behind its remote counterpart
   - **Warn** if the branch has unpushed commits
   - **Delete** if the upstream was removed and the branch was merged (or squash-merged) into the default branch
   - **Warn** if the upstream was removed but the branch appears unmerged
   - **Skip** a branch that would be deleted if it is checked out in another worktree, since git can't delete it

Messages about skipped or unmerged branches go to stderr.

Branches without explicit tracking configuration are matched by name against the remote.

### ⚙️ Flags

```
--verbose, -v     Log each git command to stderr
--dry-run, -n     Preview changes; no branch is moved or deleted
```

`--dry-run` still runs `git fetch --prune`, because deleted upstreams are only detectable after remote-tracking refs are pruned. It cannot predict failures that only surface at mutation time (e.g. `merge --ff-only` refusing on a dirty working tree).

### 📝 Examples

Fast-forward a branch that fell behind:

```
Updated branch feature-login (was 3a1b2c4).
```

Warn about local work that hasn't been pushed:

```
warning: 'experiment' seems to contain unpushed commits
```

Clean up a merged branch whose remote was deleted:

```
Deleted branch add-user-api (was 7f8e9d0).
```

Preview the same run with `--dry-run`:

```
Would update branch feature-login (3a1b2c4 -> 9c8b7a6).
Would delete branch add-user-api (was 7f8e9d0): merged into main
```

Skip a branch that is checked out in another worktree (git can't delete it):

```
skipped add-user-api: checked out in worktree /home/me/work/add-user-api
```

If such a branch is unmerged, the usual warning is followed by a note:

```
warning: 'experiment' was deleted on origin, but appears not merged into 'main'
note: 'experiment' is checked out in worktree /home/me/work/experiment
```

### 📦 Upgrade / Uninstall

```shell
gh extension upgrade looztra/gh-sync
gh extension remove gh-sync
```

### 🧪 Development

Requires Go (see `go.mod` for the version). Run the test suite with:

```shell
go test ./...
```

To try a local build without releasing, either run the binary directly or install the checkout as the extension.

**Run the binary directly.** `gh sync` just executes a `gh-sync` binary, so you can skip `gh` entirely. Run it from the repository you want to sync, since it operates on the current directory:

```shell
go build -o /tmp/gh-sync .
cd /path/to/some/repo && /tmp/gh-sync --verbose
```

**Install the checkout as the extension.** If a released `gh sync` is already installed, remove it first:

```shell
gh extension remove sync
go build -o gh-sync .
gh extension install .
gh sync
```

The extension is linked to the checkout directory, so after code changes just run `go build -o gh-sync .` again. The `gh-sync` binary is git-ignored. To go back to the released version:

```shell
gh extension remove sync
gh extension install looztra/gh-sync
```

`gh extension install <owner>/<repo>` cannot install an unreleased branch, because this is a precompiled extension and `gh` only downloads release assets.

### 🙏 Attribution

Inspired by [`jacobwgillespie/git-sync`](https://github.com/jacobwgillespie/git-sync), rebuilt as a `gh` CLI extension by [`wassimk/gh-sync`](https://github.com/wassimk/gh-sync), which this repo is forked from and now maintained independently.
