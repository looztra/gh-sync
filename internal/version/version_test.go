package version

import (
	"strings"
	"testing"
)

func TestDescribe(t *testing.T) {
	const rev = "af64d172b980c1b2f1a5d3e4c5b6a7980e1f2a3b"
	const ts = "2026-10-17T15:12:50Z"

	tests := []struct {
		name     string
		tag      string
		branch   string
		revision string
		vcsTime  string
		modified bool
		want     Info
	}{
		{"tag", "v1.0.0", "main", rev, ts, false, Info{"v1.0.0", "af64d17", ts}},
		{"main no tag", "", "main", rev, ts, false, Info{"main", "af64d17", ts}},
		{"branch slug", "", "feat/VERSION flag", rev, ts, false, Info{"feat-version-flag", "af64d17", ts}},
		{"no tag no branch", "", "", rev, ts, false, Info{"dev", "af64d17", ts}},
		{"modified", "", "main", rev, ts, true, Info{"main", "af64d17-dirty", ts}},
		{"empty revision and time", "", "main", "", "", false, Info{"main", "unknown", "unknown"}},
		{"empty revision modified", "", "main", "", ts, true, Info{"main", "unknown", ts}},
		{"bad time", "", "main", rev, "yesterday", false, Info{"main", "af64d17", "unknown"}},
		{"offset time", "", "main", rev, "2026-10-17T17:12:50+02:00", false, Info{"main", "af64d17", ts}},
		{"short revision", "", "main", "abc", ts, false, Info{"main", "abc", ts}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describe(tt.tag, tt.branch, tt.revision, tt.vcsTime, tt.modified)
			if got != tt.want {
				t.Errorf("describe() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSlug(t *testing.T) {
	long := strings.Repeat("a", 49) + "/" + strings.Repeat("b", 30)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "main", "main"},
		{"slash", "feat/version-flag", "feat-version-flag"},
		{"mixed", "Feature/ABC-123_Fix Thing", "feature-abc-123-fix-thing"},
		{"only separators", "---", "unknown"},
		{"empty", "", "unknown"},
		{"truncated, no trailing dash", long, strings.Repeat("a", 49)},
		{"80 chars", strings.Repeat("x", 80), strings.Repeat("x", 50)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Slug(tt.in); got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
