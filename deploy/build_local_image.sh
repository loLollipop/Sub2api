#!/usr/bin/env bash
# Build the customized frontend with this repository's existing backend.
# Publishing is opt-in and restricted to the maintainer's GHCR namespace.
set -euo pipefail

if [[ "${1:-}" == "--help" ]]; then
    printf '%s\n' \
        'Build: IMAGE_NAME=ghcr.io/lolollipop/sub2api:frontend-local ./deploy/build_local_image.sh' \
        'Publish: PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:<version> ./deploy/build_local_image.sh' \
        'Options: PUSH=false, PLATFORM=linux/amd64, BUILDER_NAME=sub2api-local-frontend' \
        'Uses the root Dockerfile. No login, deployment, image deletion or cache pruning is performed.'
    exit 0
fi
if [[ $# -ne 0 ]]; then
    printf 'Unknown argument. Use --help.\n' >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
IMAGE_NAME="${IMAGE_NAME:-ghcr.io/lolollipop/sub2api:frontend-local}"
PUSH="${PUSH:-false}"
PLATFORM="${PLATFORM:-linux/amd64}"
BUILDER_NAME="${BUILDER_NAME:-sub2api-local-frontend}"

if [[ "${PUSH}" != "true" && "${PUSH}" != "false" ]]; then
    printf 'PUSH must be true or false.\n' >&2
    exit 1
fi
if [[ ! "${IMAGE_NAME}" =~ ^[a-z0-9][a-z0-9._/:@-]*$ ]]; then
    printf 'IMAGE_NAME must be a lowercase image reference, without spaces or options.\n' >&2
    exit 1
fi
if [[ "${PUSH}" == "true" && "${IMAGE_NAME}" != ghcr.io/lolollipop/* ]]; then
    printf 'Publishing is restricted to ghcr.io/lolollipop/.\n' >&2
    exit 1
fi
if [[ ! "${BUILDER_NAME}" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]]; then
    printf 'Invalid BUILDER_NAME.\n' >&2
    exit 1
fi
if [[ ! "${PLATFORM}" =~ ^linux/(amd64|arm64)(,linux/(amd64|arm64))*$ ]]; then
    printf 'PLATFORM must be linux/amd64, linux/arm64 or a comma-separated list of them.\n' >&2
    exit 1
fi
if [[ "${PUSH}" == "false" && "${PLATFORM}" == *,* ]]; then
    printf 'Local --load builds require one platform; use PUSH=true for multi-platform publishing.\n' >&2
    exit 1
fi

command -v docker >/dev/null || { printf 'Docker with buildx is required.\n' >&2; exit 1; }
docker buildx version >/dev/null
if ! docker buildx inspect "${BUILDER_NAME}" >/dev/null 2>&1; then
    # Do not change the user's selected builder with --use.
    docker buildx create --name "${BUILDER_NAME}" --driver docker-container >/dev/null
fi

COMMIT="${COMMIT:-$(git -C "${REPO_ROOT}" rev-parse HEAD)}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
if [[ -n "$(git -C "${REPO_ROOT}" status --porcelain --untracked-files=normal)" ]]; then
    COMMIT="${COMMIT}-dirty"
    printf 'Warning: source contains uncommitted changes; image revision is marked dirty.\n' >&2
fi

BUILD_ARGS=(
    --builder "${BUILDER_NAME}"
    --platform "${PLATFORM}"
    -t "${IMAGE_NAME}"
    -f "${REPO_ROOT}/Dockerfile"
    --build-arg "COMMIT=${COMMIT}"
    --build-arg "DATE=${DATE}"
)
# Only explicit, non-secret overrides are passed to the Dockerfile.
for ARG_NAME in NODE_IMAGE GOLANG_IMAGE ALPINE_IMAGE POSTGRES_IMAGE GOPROXY GOSUMDB NPM_CONFIG_REGISTRY VERSION; do
    if [[ -n "${!ARG_NAME:-}" ]]; then
        BUILD_ARGS+=(--build-arg "${ARG_NAME}=${!ARG_NAME}")
    fi
done
if [[ "${PUSH}" == "true" ]]; then
    BUILD_ARGS+=(--push)
else
    BUILD_ARGS+=(--load)
fi

docker buildx build "${BUILD_ARGS[@]}" "${REPO_ROOT}"
