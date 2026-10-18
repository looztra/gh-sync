// Package version resolves the build identity shown by `gh sync --version`.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// Injected at build time with -X.
var (
	Tag    string // exact release tag, e.g. "v1.0.0"
	Branch string // branch name, e.g. "main" or "feat/version-flag"
)

const (
	unknown   = "unknown"
	shortSHA  = 7
	slugLimit = 50
)

// Info is the resolved build identity.
type Info struct {
	Ref    string // release tag, branch slug (e.g. "main"), or "dev"
	Commit string // 7-char sha, optionally "-dirty"; "unknown" if absent
	Date   string // RFC3339 UTC, e.g. "2026-10-17T15:12:50Z"; "unknown" if absent
}

func describe(tag, branch, revision, vcsTime string, modified bool) Info {
	var info Info

	switch {
	case tag != "":
		info.Ref = tag
	case branch != "":
		info.Ref = Slug(branch)
	default:
		info.Ref = "dev"
	}

	switch {
	case revision == "":
		info.Commit = unknown
	case len(revision) >= shortSHA:
		info.Commit = revision[:shortSHA]
	default:
		info.Commit = revision
	}
	if modified && revision != "" {
		info.Commit += "-dirty"
	}

	info.Date = unknown
	if vcsTime != "" {
		if t, err := time.Parse(time.RFC3339, vcsTime); err == nil {
			info.Date = t.UTC().Format(time.RFC3339)
		}
	}

	return info
}

// Current returns the build identity from ldflags and Go's VCS stamping.
func Current() Info {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return describe(Tag, Branch, "", "", false)
	}
	return fromSettings(Tag, Branch, bi.Settings)
}

// fromSettings resolves the identity from the vcs.* build settings.
func fromSettings(tag, branch string, settings []debug.BuildSetting) Info {
	var revision, vcsTime string
	var modified bool
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			vcsTime = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}

	return describe(tag, branch, revision, vcsTime, modified)
}

// String formats the identity as a single line.
func (i Info) String() string {
	return fmt.Sprintf("gh-sync %s (%s) %s", i.Ref, i.Commit, i.Date)
}

// Slug lowercases s and collapses runs of non-alphanumerics into single dashes.
func Slug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}

	out := strings.Trim(b.String(), "-")
	if len(out) > slugLimit {
		out = strings.TrimRight(out[:slugLimit], "-")
	}
	if out == "" {
		return unknown
	}
	return out
}
