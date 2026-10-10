#!/usr/bin/env bash
set -Eeuo pipefail

# Deliberately small, manual updater for a binary published in loLollipop/Sub2api.
# It never builds the project, runs archive-provided scripts, or applies migrations.

CONFIG_FILE=${SUB2API_UPDATER_CONFIG:-/etc/sub2api-release-updater.conf}

die() { printf 'vps-release-update: %s\n' "$*" >&2; return 1; }
log() { printf 'vps-release-update: %s\n' "$*" >&2; }

if (( EUID != 0 )); then die 'must be run as root'; exit 1; fi
[[ -r "$CONFIG_FILE" ]] || { die "config is not readable: $CONFIG_FILE"; exit 1; }
config_uid=$(stat -c '%u' -- "$CONFIG_FILE")
config_mode=$(stat -c '%a' -- "$CONFIG_FILE")
[[ "$config_uid" == 0 ]] || { die 'config must be owned by root'; exit 1; }
(( 10#$config_mode % 10 < 2 && (10#$config_mode / 10) % 10 < 2 )) || {
  die 'config must not be group/world writable'; exit 1;
}
# shellcheck disable=SC1090
source "$CONFIG_FILE"

: "${REPOSITORY:=loLollipop/Sub2api}"
: "${SERVICE_NAME:=sub2api.service}"
: "${STATE_DIR:=/var/lib/sub2api-release-updater}"
: "${RELEASES_DIR:=/srv/apps/sub2api-releases}"
: "${DROPIN_PATH:=/etc/systemd/system/sub2api.service.d/90-managed-release.conf}"
: "${ENVIRONMENT_FILE:=/srv/apps/sub2api-releases/20261004-update-ca0602bf/runtime-production.env}"
: "${WORKING_DIRECTORY:=/srv/apps/sub2api-releases/20261004-update-ca0602bf/source/backend}"
: "${READY_URL:=http://127.0.0.1:18081/readyz}"
: "${API_BASE:=https://api.github.com}"
: "${HEALTH_ATTEMPTS:=30}"
: "${HEALTH_SUCCESS_COUNT:=3}"
: "${HEALTH_DELAY_SECONDS:=2}"
: "${STOP_TIMEOUT_SECONDS:=30}"
: "${SYSTEMCTL:=systemctl}"
: "${SYSTEMD_RUN:=systemd-run}"
: "${CURL:=curl}"
: "${TAR:=tar}"
: "${SHA256SUM:=sha256sum}"
: "${FILE:=file}"
: "${PGREP:=pgrep}"
: "${FLOCK:=flock}"
: "${JQ:=jq}"

[[ "$REPOSITORY" == 'loLollipop/Sub2api' ]] || { die 'REPOSITORY must be exactly loLollipop/Sub2api'; exit 1; }
[[ "$API_BASE" == 'https://api.github.com' ]] || { die 'API_BASE must be https://api.github.com'; exit 1; }
[[ "$HEALTH_ATTEMPTS" =~ ^[1-9][0-9]*$ && "$HEALTH_SUCCESS_COUNT" =~ ^[1-9][0-9]*$ ]] || {
  die 'health limits must be positive integers'; exit 1;
}

mkdir -p -- "$STATE_DIR" "$RELEASES_DIR"
chmod 700 -- "$STATE_DIR"
LOCK_FILE=${LOCK_FILE:-$STATE_DIR/update.lock}
if [[ ${SUB2API_UPDATER_LOCK_FD:-} == 9 ]]; then
  # A controlled outer transaction can keep this same lock through its final
  # verification/rollback. Never skip locking: verify the inherited descriptor
  # names the configured lock inode, then flock that shared open description.
  [[ -f "$LOCK_FILE" && ! -L "$LOCK_FILE" && \
     $(stat -Lc '%d:%i' -- /proc/$$/fd/9) == "$(stat -Lc '%d:%i' -- "$LOCK_FILE")" ]] || {
    die 'inherited lock descriptor does not match LOCK_FILE'; exit 1;
  }
elif [[ -n ${SUB2API_UPDATER_LOCK_FD:-} ]]; then
  die 'only inherited lock descriptor 9 is supported'; exit 1
else
  exec 9>"$LOCK_FILE"
fi
"$FLOCK" -n 9 || { die 'another update is already running'; exit 1; }

usage() { printf 'usage: %s [--expected-binary-sha256 <64hex>] [latest|tag]\n' "$0"; }
requested=latest
requested_set=0
expected_binary_sha256=
while (( $# )); do
  case "$1" in
    --expected-binary-sha256)
      [[ $# -ge 2 && -z "$expected_binary_sha256" && "$2" =~ ^[0-9A-Fa-f]{64}$ ]] || {
        die '--expected-binary-sha256 requires one 64-character hexadecimal SHA256'; exit 2;
      }
      expected_binary_sha256=${2,,}
      shift 2
      ;;
    *)
      (( requested_set == 0 )) || { usage >&2; exit 2; }
      requested=$1
      requested_set=1
      shift
      ;;
  esac
done
if [[ "$requested" != latest && ! "$requested" =~ ^v?[0-9][0-9A-Za-z._-]*$ ]]; then
  die 'tag must contain only an optional v followed by version-safe characters'; exit 2
fi

tmp_root=$(mktemp -d "$STATE_DIR/.update.XXXXXX")
chmod 700 -- "$tmp_root"
staging_dir=
cleanup_tmp() {
  rm -rf -- "$tmp_root"
  if [[ -n "$staging_dir" && -e "$staging_dir" ]]; then
    rm -rf -- "$staging_dir"
  fi
}
trap cleanup_tmp EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

require_cmd() { command -v "$1" >/dev/null 2>&1 || { die "missing command: $1"; return 1; }; }
for cmd in "$CURL" "$TAR" "$SHA256SUM" "$FILE" "$PGREP" "$FLOCK" "$JQ" "$SYSTEMCTL" "$SYSTEMD_RUN"; do require_cmd "$cmd"; done

allowed_host() {
  local url=$1 host
  host=$(printf '%s\n' "$url" | sed -nE 's#^https://([^/:]+)(:[0-9]+)?/.*#\1#p')
  case "$host" in
    github.com|api.github.com|objects.githubusercontent.com|github-releases.githubusercontent.com|release-assets.githubusercontent.com) return 0 ;;
    *) return 1 ;;
  esac
}
asset_url_allowed() {
  local url=$1
  allowed_host "$url" || return 1
  case "$url" in
    https://github.com/loLollipop/Sub2api/releases/download/*|\
    https://api.github.com/repos/loLollipop/Sub2api/releases/assets/*|\
    https://objects.githubusercontent.com/*|\
    https://github-releases.githubusercontent.com/*|\
    https://release-assets.githubusercontent.com/*) return 0 ;;
    *) return 1 ;;
  esac
}

# Follow only redirects whose next host is an explicitly permitted GitHub host.
download_github() {
  local url=$1 output=$2 headers code http_code location i
  allowed_host "$url" || { die "refusing non-GitHub URL: $url"; return 1; }
  for ((i=0; i<6; i++)); do
    headers="$tmp_root/headers.$i"
    set +e
    http_code=$("$CURL" --proto '=https' --tlsv1.2 --silent --show-error \
      --max-redirs 0 --dump-header "$headers" --output "$output" \
      --write-out '%{http_code}' "$url")
    code=$?
    set -e
    (( code == 0 )) || { die "download failed: $url"; return 1; }
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then return 0; fi
    [[ "$http_code" =~ ^3[0-9][0-9]$ ]] || {
      die "download returned HTTP $http_code: $url"; return 1;
    }
    location=$(awk 'BEGIN{IGNORECASE=1} /^Location:[[:space:]]*/ {sub(/^[^:]*:[[:space:]]*/, ""); gsub(/[\r\n]/, ""); print; exit}' "$headers")
    [[ -n "$location" ]] || { die 'redirect had no Location'; return 1; }
    [[ "$location" == https://* ]] || { die 'redirect was not https'; return 1; }
    allowed_host "$location" || { die "refusing redirect to non-GitHub host: $location"; return 1; }
    url=$location
  done
  die 'too many redirects'
}

release_json="$tmp_root/release.json"
if [[ "$requested" == latest ]]; then
  api_url="$API_BASE/repos/$REPOSITORY/releases/latest"
else
  api_url="$API_BASE/repos/$REPOSITORY/releases/tags/$requested"
fi
download_github "$api_url" "$release_json"
release_tag=$("$JQ" -er '.tag_name // empty' "$release_json")
draft=$("$JQ" -r '.draft' "$release_json")
prerelease=$("$JQ" -r '.prerelease' "$release_json")
[[ "$draft" == false && "$prerelease" == false ]] || { die 'draft and prerelease releases are not deployable'; exit 1; }
[[ "$release_tag" =~ ^v?[0-9][0-9A-Za-z._-]*$ ]] || { die 'release tag has an unsafe format'; exit 1; }
[[ "$requested" == latest || "$requested" == "$release_tag" ]] || { die 'API returned a different tag'; exit 1; }
asset_tag=${release_tag#v}
package_name="sub2api_${asset_tag}_linux_amd64.tar.gz"
package_url=$("$JQ" -er --arg n "$package_name" '[.assets[] | select(.name == $n) | .browser_download_url] | if length == 1 then .[0] else error("asset count") end' "$release_json")
checksums_url=$("$JQ" -er '[.assets[] | select(.name == "checksums.txt") | .browser_download_url] | if length == 1 then .[0] else error("checksums asset count") end' "$release_json")
[[ -n "$package_url" && -n "$checksums_url" ]] || { die 'release lacks required assets'; exit 1; }
asset_url_allowed "$package_url" && asset_url_allowed "$checksums_url" || {
  die 'release asset URL is not a permitted loLollipop/Sub2api GitHub URL'; exit 1;
}

package_file="$tmp_root/$package_name"
checksums_file="$tmp_root/checksums.txt"
download_github "$package_url" "$package_file"
download_github "$checksums_url" "$checksums_file"
expected_hash=$(awk -v n="$package_name" 'NF == 2 && $2 == n { if (++nmatch > 1) exit 2; print tolower($1) } END { if (nmatch != 1) exit 3 }' "$checksums_file") || {
  die 'checksums.txt has no unique checksum for the release asset'; exit 1;
}
[[ "$expected_hash" =~ ^[0-9a-f]{64}$ ]] || { die 'invalid SHA256 in checksums.txt'; exit 1; }
actual_hash=$("$SHA256SUM" "$package_file" | awk '{print tolower($1)}')
[[ "$actual_hash" == "$expected_hash" ]] || { die 'release archive checksum mismatch'; exit 1; }

# Validate every member before extraction. Only ordinary files and directories
# are accepted; in particular, links cannot be used to escape the destination.
archive_listing="$tmp_root/archive.list"
"$TAR" -tvzf "$package_file" >"$archive_listing"
while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  case "${line:0:1}" in -|d) ;; *) die "archive contains a non-file/non-directory member"; exit 1 ;; esac
done <"$archive_listing"
while IFS= read -r entry; do
  [[ -n "$entry" ]] || continue
  case "$entry" in /*|../*|*/../*|..|*'\'* ) die "archive contains an unsafe path: $entry"; exit 1 ;; esac
done < <("$TAR" -tzf "$package_file")

extract_root="$tmp_root/extract"
mkdir -m 700 -- "$extract_root"
"$TAR" --no-same-owner --no-same-permissions -xzf "$package_file" -C "$extract_root"
mapfile -t binaries < <(find "$extract_root" -type f -name sub2api -print)
(( ${#binaries[@]} == 1 )) || { die 'archive must contain exactly one regular sub2api file'; exit 1; }
source_binary=${binaries[0]}
chmod 0755 -- "$source_binary"
file_description=$("$FILE" -b -- "$source_binary")
[[ "$file_description" == *ELF* && "$file_description" == *x86-64* ]] || {
  die "candidate is not an ELF Linux amd64 binary: $file_description"; exit 1;
}
source_binary_hash=$("$SHA256SUM" "$source_binary" | awk '{print tolower($1)}')
[[ "$source_binary_hash" =~ ^[0-9a-f]{64}$ ]] || { die 'candidate binary hash is invalid'; exit 1; }
[[ -z "$expected_binary_sha256" || "$source_binary_hash" == "$expected_binary_sha256" ]] || {
  die 'candidate binary does not match --expected-binary-sha256'; exit 1;
}

release_dir="$RELEASES_DIR/$release_tag"
candidate="$release_dir/sub2api"
if [[ -e "$release_dir" ]]; then
  [[ -f "$candidate" && ! -L "$candidate" ]] || {
    die "existing release directory is incomplete: $release_dir"; exit 1;
  }
  existing_binary_hash=$("$SHA256SUM" "$candidate" | awk '{print tolower($1)}')
  [[ "$existing_binary_hash" == "$source_binary_hash" ]] || {
    die "existing release differs from the immutable GitHub asset: $release_dir"; exit 1;
  }
fi
# Verify controlled deployment identity and any committed immutable release
# before invoking even the candidate's version or migration checks.
"$source_binary" -version >/dev/null || { die 'candidate -version check failed'; exit 1; }
if [[ ! -e "$release_dir" ]]; then
  # Build the complete immutable release on the same filesystem, then publish
  # it with one rename. EXIT (including handled signals) removes only an
  # uncommitted staging directory.
  staging_dir=$(mktemp -d "$RELEASES_DIR/.${release_tag}.staging.XXXXXX")
  chmod 0755 -- "$staging_dir"
  staged_candidate="$staging_dir/sub2api"
  install -m 0755 -- "$source_binary" "$staged_candidate"
  staged_binary_hash=$("$SHA256SUM" "$staged_candidate" | awk '{print tolower($1)}')
  [[ "$staged_binary_hash" == "$source_binary_hash" ]] || {
    die 'installed candidate checksum mismatch'; exit 1
  }
  sync -f "$staged_candidate"
  sync -f "$staging_dir"
  mv -T -- "$staging_dir" "$release_dir"
  staging_dir=
  sync -f "$RELEASES_DIR"
fi
candidate=$(readlink -f -- "$candidate")

txn_id="$(date -u +%Y%m%dT%H%M%SZ)-$$-$release_tag"
txn_dir="$STATE_DIR/transactions/$txn_id"
mkdir -m 700 -p -- "$txn_dir"
if [[ -e "$DROPIN_PATH" ]]; then
  cp -p -- "$DROPIN_PATH" "$txn_dir/previous-dropin"
  printf 'present\n' >"$txn_dir/previous-dropin.state"
else
  printf 'absent\n' >"$txn_dir/previous-dropin.state"
fi
old_pid=$("$SYSTEMCTL" show -p MainPID --value "$SERVICE_NAME")
[[ "$old_pid" =~ ^[1-9][0-9]*$ ]] || { die 'could not determine current MainPID'; exit 1; }
old_exe=$(readlink -f -- "/proc/$old_pid/exe")
[[ -x "$old_exe" ]] || { die 'current MainPID executable is unavailable'; exit 1; }
old_hash=$("$SHA256SUM" "$old_exe" | awk '{print $1}')
printf 'service=%s\nold_pid=%s\nold_exe=%s\nold_hash=%s\nnew_exe=%s\nnew_hash=%s\narchive_hash=%s\ntag=%s\n' \
  "$SERVICE_NAME" "$old_pid" "$old_exe" "$old_hash" "$candidate" "$source_binary_hash" "$actual_hash" "$release_tag" >"$txn_dir/state"

migration_unit="sub2api-migration-check-$$"
"$SYSTEMD_RUN" --quiet --wait --collect --unit="$migration_unit" \
  --property="Type=oneshot" --property="WorkingDirectory=$WORKING_DIRECTORY" \
  --property="EnvironmentFile=$ENVIRONMENT_FILE" -- "$candidate" -check-migrations || {
  die 'candidate migration check failed; service was not stopped'; exit 1
}

TRANSACTION_ACTIVE=0
ROLLING_BACK=0
rollback() {
  local rc=$1 rollback_failed=0
  ROLLING_BACK=1
  TRANSACTION_ACTIVE=0
  trap - ERR INT TERM HUP
  set +e
  log 'update failed; restoring the previous service instance'
  if [[ -f "$txn_dir/previous-dropin.state" && $(<"$txn_dir/previous-dropin.state") == present ]]; then
    atomic_copy "$txn_dir/previous-dropin" "$DROPIN_PATH" || {
      log 'rollback failed: could not restore the previous systemd drop-in'
      rollback_failed=1
    }
  else
    rm -f -- "$DROPIN_PATH" || {
      log 'rollback failed: could not remove the candidate systemd drop-in'
      rollback_failed=1
    }
  fi
  "$SYSTEMCTL" stop "$SERVICE_NAME" || {
    log 'rollback failed: could not stop the candidate service'
    rollback_failed=1
  }
  wait_stopped || {
    log 'rollback failed: the candidate process is still running'
    rollback_failed=1
  }
  "$SYSTEMCTL" daemon-reload || {
    log 'rollback failed: systemd daemon-reload failed'
    rollback_failed=1
  }
  "$SYSTEMCTL" start "$SERVICE_NAME" || {
    log 'rollback failed: could not start the previous service'
    rollback_failed=1
  }
  if (( rollback_failed == 0 )); then
    wait_ready "$old_exe" "$old_hash" 'restored service' || rollback_failed=1
  fi
  if (( rollback_failed != 0 )); then
    log "rollback failed: previous executable $old_exe was not restored and verified"
  else
    log "rollback succeeded: restored $old_exe with sha256 $old_hash"
  fi
  set -e
  (( rollback_failed == 0 )) || return 1
  return "$rc"
}
on_error() { local rc=$?; (( TRANSACTION_ACTIVE )) && rollback "$rc"; exit "$rc"; }
on_signal() { local sig=$1; (( TRANSACTION_ACTIVE )) && rollback 130; trap - "$sig"; kill -s "$sig" "$$"; }
trap on_error ERR
trap 'on_signal INT' INT
trap 'on_signal TERM' TERM
trap 'on_signal HUP' HUP

atomic_copy() {
  local source=$1 destination=$2 dir tmp=
  dir=$(dirname -- "$destination")
  mkdir -p -- "$dir" || return 1
  tmp=$(mktemp "$dir/.90-managed-release.conf.XXXXXX") || return 1
  if ! chmod 0644 -- "$tmp" || ! cp -- "$source" "$tmp"; then
    rm -f -- "$tmp"
    return 1
  fi
  sync -f "$tmp" 2>/dev/null || true
  if ! mv -f -- "$tmp" "$destination"; then
    rm -f -- "$tmp"
    return 1
  fi
  sync -f "$dir" 2>/dev/null || true
}
write_dropin() {
  local dir tmp
  dir=$(dirname -- "$DROPIN_PATH")
  mkdir -p -- "$dir"
  tmp=$(mktemp "$dir/.90-managed-release.conf.XXXXXX")
  chmod 0644 -- "$tmp"
  {
    printf '[Service]\n'
    printf 'WorkingDirectory=%s\n' "$WORKING_DIRECTORY"
    printf 'EnvironmentFile=%s\n' "$ENVIRONMENT_FILE"
    printf 'ExecStart=\nExecStart=%s\n' "$candidate"
  } >"$tmp"
  sync -f "$tmp" 2>/dev/null || true
  mv -f -- "$tmp" "$DROPIN_PATH"
  sync -f "$dir" 2>/dev/null || true
}
wait_stopped() {
  local i pid
  for ((i=0; i<STOP_TIMEOUT_SECONDS; i++)); do
    pid=$("$SYSTEMCTL" show -p MainPID --value "$SERVICE_NAME" 2>/dev/null || true)
    [[ "$pid" == 0 || -z "$pid" ]] && return 0
    sleep 1
  done
  die 'old service process did not exit'; return 1
}
wait_ready() {
  local expected=$1 expected_hash=$2 description=${3:-service} i pid exe actual_hash count=0
  for ((i=0; i<HEALTH_ATTEMPTS; i++)); do
    if "$SYSTEMCTL" is-active --quiet "$SERVICE_NAME" && \
       pid=$("$SYSTEMCTL" show -p MainPID --value "$SERVICE_NAME") && [[ "$pid" =~ ^[1-9][0-9]*$ ]] && \
       exe=$(readlink -f -- "/proc/$pid/exe") && [[ "$exe" == "$expected" ]] && \
       actual_hash=$("$SHA256SUM" "$exe" | awk '{print tolower($1)}') && [[ "$actual_hash" == "$expected_hash" ]] && \
       "$CURL" --fail --silent --show-error --max-time 5 "$READY_URL" >/dev/null 2>&1 && \
       [[ $("$PGREP" -x sub2api | wc -l) -eq 1 ]]; then
      ((count += 1))
      (( count >= HEALTH_SUCCESS_COUNT )) && return 0
    else
      count=0
    fi
    sleep "$HEALTH_DELAY_SECONDS"
  done
  die "$description did not become healthy with the expected executable and checksum"; return 1
}

TRANSACTION_ACTIVE=1
"$SYSTEMCTL" stop "$SERVICE_NAME"
wait_stopped
write_dropin
"$SYSTEMCTL" daemon-reload
"$SYSTEMCTL" start "$SERVICE_NAME"
wait_ready "$candidate" "$source_binary_hash" 'candidate service'

cat >"$STATE_DIR/last-good" <<EOF
tag=$release_tag
release_dir=$release_dir
binary=$candidate
sha256=$source_binary_hash
archive_sha256=$actual_hash
installed_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF
chmod 0600 -- "$STATE_DIR/last-good"
cp -- "$txn_dir/state" "$STATE_DIR/installed"
chmod 0600 -- "$STATE_DIR/installed"
TRANSACTION_ACTIVE=0
log "installed $release_tag successfully"
