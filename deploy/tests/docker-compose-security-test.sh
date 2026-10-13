#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

check_application_security_opt() {
  file=$1
  count=$(
    awk '
      $0 == "  sub2api:" {
        in_application = 1
        next
      }
      in_application && $0 ~ /^  [A-Za-z0-9_-]+:$/ {
        in_application = 0
      }
      in_application && $0 == "    security_opt:" {
        in_security_opt = 1
        next
      }
      in_application && in_security_opt && $0 == "      - no-new-privileges:true" {
        count++
      }
      END { print count + 0 }
    ' "$file"
  )

  if [ "$count" -ne 1 ]; then
    printf '%s must enable no-new-privileges exactly once for the sub2api service\n' "$file" >&2
    exit 1
  fi
}

for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml \
  deploy/docker-compose.dev.yml
do
  check_application_security_opt "$compose_file"
  # An empty login email must reach the backend's random bootstrap generator.
  # Compose still honors an explicitly configured email on fresh installations.
  admin_email_count=$(grep -Fxc '      - ADMIN_EMAIL=${ADMIN_EMAIL:-}' "$compose_file" || true)
  admin_email_total=$(grep -Ec '^[[:space:]]*-[[:space:]]*ADMIN_EMAIL=' "$compose_file" || true)
  if [ "$admin_email_count" -ne 1 ] || [ "$admin_email_total" -ne 1 ]; then
    printf '%s must pass ADMIN_EMAIL with an empty fallback exactly once\n' "$compose_file" >&2
    exit 1
  fi
done

if [ "$(grep -c '^ADMIN_EMAIL=' deploy/.env.example)" -ne 1 ] || \
   ! grep -qx 'ADMIN_EMAIL=' deploy/.env.example; then
  printf 'env example must leave ADMIN_EMAIL empty for randomized bootstrap\n' >&2
  exit 1
fi

printf 'docker compose security test passed\n'
