#!/usr/bin/env bash
# Build the customized frontend and backend with our maintained Dockerfile.
set -euo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
PUSH=${PUSH:-false}
PLATFORM=${PLATFORM:-linux/amd64}
CLEAN_AFTER_PUSH=${CLEAN_AFTER_PUSH:-true}
for boolean in "$PUSH" "$CLEAN_AFTER_PUSH"; do
  [[ "$boolean" == true || "$boolean" == false ]] || { echo 'Boolean options must be true or false' >&2; exit 1; }
done
[[ "$PLATFORM" =~ ^linux/(amd64|arm64)(,linux/(amd64|arm64))*$ ]] || { echo 'Supported platforms: linux/amd64, linux/arm64' >&2; exit 1; }
if [[ "$PUSH" == false && "$PLATFORM" == *,* ]]; then
  echo 'Multi-platform builds require PUSH=true; --load supports one platform' >&2
  exit 1
fi

COMMIT=$(git -C "$ROOT" rev-parse --verify HEAD)
VERSION=${VERSION:-$(tr -d '\r\n' < "$ROOT/backend/cmd/server/VERSION")}
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
IMAGE_NAME=${IMAGE_NAME:-ghcr.io/lolollipop/sub2api:local-${COMMIT:0:12}}
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.+-][a-zA-Z0-9.-]+)?$ ]] || { echo 'Invalid VERSION' >&2; exit 1; }
[[ "$IMAGE_NAME" =~ ^ghcr\.io/lolollipop/sub2api:[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || {
  echo 'IMAGE_NAME must be a tagged image in ghcr.io/lolollipop/sub2api' >&2; exit 1;
}
[[ -z "$(git -C "$ROOT" status --porcelain --untracked-files=all)" ]] || {
  echo 'Commit source changes before building an image' >&2; exit 1;
}
if [[ "$PUSH" == true ]]; then
  # Stable tags and latest belong to the verified release workflow.
  image_tag=${IMAGE_NAME##*:}
  [[ "$image_tag" == "local-${COMMIT:0:12}" || "$image_tag" == "local-${COMMIT:0:12}-amd64" || "$image_tag" == "local-${COMMIT:0:12}-arm64" ]] || {
    echo 'Use a local-<commit> tag; stable tags are published by Release CI' >&2; exit 1;
  }
fi

docker buildx version >/dev/null
owned_builder=false
build_succeeded=false
context=
cleanup() {
  # Never prune or remove a caller-provided/shared builder.
  if [[ "$owned_builder" == true && "$build_succeeded" == true && "$CLEAN_AFTER_PUSH" == true ]]; then
    docker buildx rm "$builder" >/dev/null || echo "Could not remove task builder: $builder" >&2
  fi
  if [[ -n "$context" && "$context" == "$context_parent"/sub2api-build.* && -d "$context" && ! -L "$context" ]]; then
    rm -rf -- "$context"
  fi
}
trap cleanup EXIT

# Git's archive supplies exactly the reviewed commit, excluding ignored credentials,
# local artifacts and any worktree changes made after the clean check above.
context_parent=$(cd -- "${TMPDIR:-/tmp}" && pwd)
context=$(mktemp -d "$context_parent/sub2api-build.XXXXXXXX")
git -C "$ROOT" archive "$COMMIT" | tar -xf - -C "$context"
args=(buildx build --file "$context/Dockerfile" --platform "$PLATFORM" --tag "$IMAGE_NAME"
  --build-arg "VERSION=$VERSION" --build-arg "COMMIT=$COMMIT" --build-arg "DATE=$DATE"
  --label "org.opencontainers.image.source=https://github.com/loLollipop/Sub2api"
  --label "org.opencontainers.image.revision=$COMMIT")
if [[ "$PUSH" == true ]]; then
  if [[ -n "${BUILDER_NAME:-}" ]]; then
    builder=$BUILDER_NAME
    docker buildx inspect "$builder" >/dev/null
  else
    builder="sub2api-local-${COMMIT:0:12}-$$"
    docker buildx create --name "$builder" --driver docker-container >/dev/null
    owned_builder=true
  fi
  args+=(--builder "$builder" --push)
else
  args+=(--load)
fi

# Optional registry/mirror settings; credentials must never be build arguments.
for option in NODE_IMAGE GOLANG_IMAGE ALPINE_IMAGE POSTGRES_IMAGE GOPROXY GOSUMDB NPM_CONFIG_REGISTRY; do
  if [[ -n "${!option:-}" ]]; then args+=(--build-arg "$option=${!option}"); fi
done
printf 'Building %s (%s), commit %s\n' "$IMAGE_NAME" "$PLATFORM" "$COMMIT"
docker "${args[@]}" "$context"
build_succeeded=true
printf 'Image ready: %s\n' "$IMAGE_NAME"
