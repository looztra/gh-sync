#!/usr/bin/env bash
set -euo pipefail

pkg="github.com/looztra/gh-sync/internal/version"
tag="$(git describe --tags --exact-match --match 'v*' 2>/dev/null || true)"
branch="$(git symbolic-ref --short -q HEAD 2>/dev/null || true)"

# Go splits -ldflags on quotes, so replace any in the values before quoting them.
tag="${tag//[\'\"]/-}"
branch="${branch//[\'\"]/-}"

go build -trimpath \
  -ldflags="-s -w -X '${pkg}.Tag=${tag}' -X '${pkg}.Branch=${branch}'" \
  -o gh-sync .
