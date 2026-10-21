#!/usr/bin/env bash
# Exercise publication boundaries without a daemon or registry credentials.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf -- "$fixture"' EXIT
mkdir -p "$fixture/source/deploy" "$fixture/source/backend/cmd/server" "$fixture/bin"
cp "$root/deploy/build_local_image.sh" "$fixture/source/deploy/"
printf 'FROM scratch\n' > "$fixture/source/Dockerfile"
printf '2.1.29\n' > "$fixture/source/backend/cmd/server/VERSION"
printf 'credentials*.json\n' > "$fixture/source/.gitignore"
git -C "$fixture/source" init -q
git -C "$fixture/source" add .
git -C "$fixture/source" -c user.name=Test -c user.email=test@example.invalid commit -qm fixture
printf 'ignored-secret-fixture\n' > "$fixture/source/credentials.private.json"
cat > "$fixture/bin/docker" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "CALL" "$@" >> "$DOCKER_TEST_LOG"
if [[ "${1:-} ${2:-}" == 'buildx build' ]]; then
  context=${!#}
  [[ ! -e "$context/credentials.private.json" && -s "$context/Dockerfile" ]] || exit 9
fi
if [[ "${1:-} ${2:-}" == 'buildx build' && "${FAIL_BUILD:-false}" == true ]]; then exit 7; fi
MOCK
chmod +x "$fixture/bin/docker"
export PATH="$fixture/bin:$PATH" DOCKER_TEST_LOG="$fixture/docker.log"
script="$fixture/source/deploy/build_local_image.sh"
fail() { echo "build local image test failed: $*" >&2; exit 1; }
reject() {
  : > "$DOCKER_TEST_LOG"
  if env "$@" bash "$script" > "$fixture/output" 2>&1; then fail "accepted $*"; fi
  [[ ! -s "$DOCKER_TEST_LOG" ]] || fail 'rejected request reached Docker'
}
reject PUSH=true IMAGE_NAME=ghcr.io/someone-else/sub2api:local-test
reject PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:latest
reject PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:2.1.28
reject PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:prod
reject PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:local-wrongcommit
reject PUSH=false PLATFORM=linux/amd64,linux/arm64
reject PUSH=yes
printf dirty > "$fixture/source/untracked"
reject PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:local-test
reject PUSH=false
rm "$fixture/source/untracked"

: > "$DOCKER_TEST_LOG"
PUSH=false bash "$script" > "$fixture/output"
grep -Fxq -- '--load' "$DOCKER_TEST_LOG" || fail 'local output not loaded'
! grep -Fxq -- '--push' "$DOCKER_TEST_LOG" || fail 'local output pushed'
grep -Fxq -- "COMMIT=$(git -C "$fixture/source" rev-parse HEAD)" "$DOCKER_TEST_LOG" || fail 'missing commit provenance'
grep -Fxq -- 'VERSION=2.1.29' "$DOCKER_TEST_LOG" || fail 'missing version provenance'

: > "$DOCKER_TEST_LOG"
PUSH=true PLATFORM=linux/amd64,linux/arm64 bash "$script" > "$fixture/output"
grep -Fxq -- '--push' "$DOCKER_TEST_LOG" || fail 'publish not pushed'
grep -Fxq -- 'create' "$DOCKER_TEST_LOG" || fail 'missing isolated builder'
grep -Fxq -- 'rm' "$DOCKER_TEST_LOG" || fail 'isolated cache not cleaned'
! grep -Fxq -- 'prune' "$DOCKER_TEST_LOG" || fail 'global/shared cache pruned'

: > "$DOCKER_TEST_LOG"
PUSH=true BUILDER_NAME=shared bash "$script" > "$fixture/output"
! grep -Fxq -- 'rm' "$DOCKER_TEST_LOG" || fail 'shared builder removed'
! grep -Fxq -- 'create' "$DOCKER_TEST_LOG" || fail 'shared builder recreated'

: > "$DOCKER_TEST_LOG"
if PUSH=true FAIL_BUILD=true bash "$script" > "$fixture/output" 2>&1; then fail 'build failure swallowed'; fi
! grep -Fxq -- 'rm' "$DOCKER_TEST_LOG" || fail 'failed build cache removed'
echo 'build local image tests passed'
