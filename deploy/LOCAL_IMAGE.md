# Customized frontend image

The customized Vue interface lives in `frontend/`. We reference qhongchen/sub2api's `dev/frontend-local` design while retaining this fork's additional pages, permissions and APIs. There is one frontend source and one maintained root `Dockerfile`; no second copy of the application or backend is needed.

On a machine with Docker Buildx, from a clean committed checkout:

```bash
docker login ghcr.io -u loLollipop  # use a token with write:packages via stdin in automation
PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:local-$(git rev-parse --short=12 HEAD) ./deploy/build_local_image.sh
```

The script defaults to `linux/amd64` and embeds the source commit, VERSION and UTC build date. `PLATFORM=linux/amd64,linux/arm64` publishes both architectures. With `PUSH=false` it loads a single-platform image into the local Docker daemon. Optional base-image and registry settings use the build arguments already supported by the root Dockerfile.

All builds require a clean checkout and use a temporary Git archive of the exact commit, keeping ignored credentials and local files outside the build context. Publishing accepts only `local-<12-character-commit>` tags (optionally suffixed `-amd64` or `-arm64`) in our own `ghcr.io/lolollipop/sub2api` package. Official version tags and `latest` are reserved for the verified Release workflow. Local publishing creates its own builder, removes that builder/cache after success by default, and retains failed build caches for diagnosis. `CLEAN_AFTER_PUSH=false` retains it. A supplied `BUILDER_NAME` is reused and never removed or pruned.

Official releases use the existing GitHub Actions workflow and its short-lived `GITHUB_TOKEN` with `packages: write`; enable repository variable `PUBLISH_GHCR=true`. The current simple-release configuration publishes Linux/amd64. Release archives and images embed the same frontend. The existing production systemd service can continue to deploy the verified runtime archive without moving its PostgreSQL, Redis or application configuration into new containers.
