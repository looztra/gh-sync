#!/usr/bin/env bash
set -euo pipefail

pkg="github.com/wassimk/gh-sync/internal/version"
tag="$(git describe --tags --exact-match 2>/dev/null || true)"
branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"

go build -trimpath \
  -ldflags="-s -w -X ${pkg}.Tag=${tag} -X ${pkg}.Branch=${branch}" \
  -o gh-sync .
