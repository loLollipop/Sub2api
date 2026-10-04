#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TASK_BUILD_LOG="$(mktemp)"
trap 'rm -f -- "$TASK_BUILD_LOG"' EXIT
export TASK_BUILD_LOG

# Export a function instead of requiring a Docker daemon or publishing images.
docker() {
    printf '%s\n' "$@" >> "$TASK_BUILD_LOG"
    if [[ "${1:-}" == buildx && "${2:-}" == inspect ]]; then
        return "${TASK_BUILDER_MISSING:-0}"
    fi
}
export -f docker

fail() { printf 'build-local-image test failed: %s\n' "$1" >&2; exit 1; }
has() { grep -Fxq -- "$1" "$TASK_BUILD_LOG" || fail "missing argument: $1"; }
absent() { if grep -Fxq -- "$1" "$TASK_BUILD_LOG"; then fail "unexpected argument: $1"; fi; }

: > "$TASK_BUILD_LOG"
bash "$ROOT_DIR/deploy/build_local_image.sh" --help >/dev/null
[[ ! -s "$TASK_BUILD_LOG" ]] || fail 'help invoked Docker'

: > "$TASK_BUILD_LOG"
PUSH=false IMAGE_NAME=sub2api:local COMMIT=test DATE=2026-10-04T00:00:00Z \
    bash "$ROOT_DIR/deploy/build_local_image.sh"
has --load
absent --push
has "$ROOT_DIR/Dockerfile"
has sub2api:local
has --builder
absent --use
absent prune
absent rm

: > "$TASK_BUILD_LOG"
PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:test PLATFORM=linux/amd64,linux/arm64 \
    TASK_BUILDER_MISSING=1 NODE_IMAGE=node:24-alpine COMMIT=test \
    bash "$ROOT_DIR/deploy/build_local_image.sh"
has create
has --push
absent --load
has ghcr.io/lolollipop/sub2api:test
has linux/amd64,linux/arm64
has NODE_IMAGE=node:24-alpine
absent --use
absent prune
absent rm
absent login

assert_rejected() {
    : > "$TASK_BUILD_LOG"
    if env "$@" bash "$ROOT_DIR/deploy/build_local_image.sh" >/dev/null 2>&1; then
        fail 'unsafe or invalid build input accepted'
    fi
    [[ ! -s "$TASK_BUILD_LOG" ]] || fail 'invalid input reached Docker'
}
assert_rejected PUSH=yes
assert_rejected PUSH=true IMAGE_NAME=ghcr.io/someone-else/sub2api:test
assert_rejected PUSH=true IMAGE_NAME=docker.io/lolollipop/sub2api:test
assert_rejected IMAGE_NAME='--output=somewhere'
assert_rejected IMAGE_NAME='ghcr.io/lolollipop/sub2api:test --push'
assert_rejected IMAGE_NAME='ghcr.io/loLollipop/sub2api:test'
assert_rejected BUILDER_NAME='--help'
assert_rejected PLATFORM=windows/amd64
assert_rejected PUSH=false PLATFORM=linux/amd64,linux/arm64

printf 'build-local-image mock checks passed\n'
