#!/usr/bin/env bash
set -Eeuo pipefail

# Offline contract tests. They use PATH-injected mocks and never contact GitHub
# or systemd on the host. The updater itself is run as root in CI/container tests.
if (( EUID != 0 )); then echo 'error: tests require root (the updater contract)' >&2; exit 1; fi
ROOT=$(cd -- "$(dirname -- "$0")/../.." && pwd)
UPDATER=$ROOT/deploy/vps-release-update.sh
TMP=$(mktemp -d)
trap 'rm -rf -- "$TMP"' EXIT
MOCK=$TMP/mock; mkdir -p "$MOCK/bin" "$MOCK/artifacts" "$MOCK/state" "$TMP/deployed"
export SUB2API_UPDATER_CONFIG=$TMP/config
cat >"$SUB2API_UPDATER_CONFIG" <<EOF
REPOSITORY='loLollipop/Sub2api'
STATE_DIR='$TMP/state'
RELEASES_DIR='$TMP/deployed'
DROPIN_PATH='$TMP/dropin/90-managed-release.conf'
ENVIRONMENT_FILE='$TMP/env'
WORKING_DIRECTORY='$TMP'
READY_URL='http://127.0.0.1:18081/readyz'
API_BASE='https://api.github.com'
HEALTH_ATTEMPTS=2
HEALTH_SUCCESS_COUNT=1
HEALTH_DELAY_SECONDS=0
STOP_TIMEOUT_SECONDS=1
EOF
chmod 600 "$SUB2API_UPDATER_CONFIG"
cat >"$TMP/env" <<'EOF'
X=1
EOF
cat >"$MOCK/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -e
out=; url=
while (($#)); do case "$1" in --output) out=$2; shift 2;; --dump-header) shift 2;; *) url=$1; shift;; esac; done
if [[ $url == http://127.0.0.1:* ]]; then
  [[ ${HEALTH_FAIL:-0} != 1 || $(<"$MOCK_SYSTEMCTL_EXE") == "$MOCK_OLD_EXE" ]]
elif [[ $url == */releases/latest || $url == */releases/tags/* ]]; then
  printf '%s\n' '{"tag_name":"v1.2.3-personal.2","draft":false,"prerelease":false,"assets":[{"name":"sub2api_1.2.3-personal.2_linux_amd64.tar.gz","browser_download_url":"https://objects.githubusercontent.com/sub2api_1.2.3-personal.2_linux_amd64.tar.gz"},{"name":"checksums.txt","browser_download_url":"https://objects.githubusercontent.com/checksums.txt"}]}' >"$out"
elif [[ $url == *checksums.txt ]]; then cp "$MOCK_RELEASE/checksums.txt" "$out"
elif [[ $url == *tar.gz ]]; then cp "$MOCK_RELEASE/package.tar.gz" "$out"
else exit 1; fi
printf '200'
EOF
cat >"$MOCK/bin/file" <<'EOF'
#!/usr/bin/env bash
echo 'ELF 64-bit LSB pie executable, x86-64'
EOF
cat >"$MOCK/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
set -e
state=${MOCK_SYSTEMCTL_STATE:?}
exe_state=${MOCK_SYSTEMCTL_EXE:?}
case "$1" in
  show) [[ $3 == MainPID* ]] && cat "$state" ;;
  is-active) [[ $(<"$state") != 0 ]] ;;
  stop)
    [[ ${STOP_FAIL:-0} != 1 ]] || exit 1
    echo 0 >"$state"
    ;;
  start)
    # systemd start is a no-op while the service is active. This is important:
    # rollback must explicitly stop the failed candidate before starting old.
    [[ $(<"$state") == 0 ]] || exit 0
    next=$(<"$MOCK_SYSTEMCTL_NEXT_PID")
    echo $((next + 1)) >"$MOCK_SYSTEMCTL_NEXT_PID"
    awk -F= '/^ExecStart=\/[^ ]/ {print $2; exit}' "$DROPIN_PATH" >"$exe_state"
    echo "$next" >"$state"
    ;;
  daemon-reload) : ;;
esac
EOF
cat >"$MOCK/bin/systemd-run" <<'EOF'
#!/usr/bin/env bash
[[ ${MIGRATION_FAIL:-0} != 1 ]] || exit 1
while (($#)); do [[ $1 == -- ]] && { shift; break; }; shift; done
"$@"
EOF
cat >"$MOCK/bin/pgrep" <<'EOF'
#!/usr/bin/env bash
echo 1
EOF
cat >"$MOCK/bin/readlink" <<'EOF'
#!/usr/bin/env bash
path=${@: -1}
if [[ $path == /proc/*/exe && ${path#/proc/} == "$(<"$MOCK_SYSTEMCTL_STATE")/exe" ]]; then
  cat "$MOCK_SYSTEMCTL_EXE"
else echo "$path"; fi
EOF
cat >"$MOCK/bin/flock" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$MOCK/bin/cp" <<'EOF'
#!/usr/bin/env bash
set -e
if [[ ${ATOMIC_COPY_FAIL:-0} == 1 && ${@: -1} == */.90-managed-release.conf.* ]]; then
  printf 'partial-copy\n' >"${@: -1}"
  exit 1
fi
exec /bin/cp "$@"
EOF
chmod +x "$MOCK/bin"/*
export PATH="$MOCK/bin:$PATH" MOCK_RELEASE="$MOCK/artifacts" MOCK_SYSTEMCTL_STATE="$TMP/pid"
export MOCK_SYSTEMCTL_EXE="$TMP/running-exe" MOCK_SYSTEMCTL_NEXT_PID="$TMP/next-pid"
echo 123 >"$MOCK_SYSTEMCTL_STATE"
printf '#!/bin/sh\n[ "$1" = -version ] || [ "$1" = -check-migrations ]\n' >"$MOCK/artifacts/sub2api"; chmod +x "$MOCK/artifacts/sub2api"
tar -czf "$MOCK/artifacts/package.tar.gz" -C "$MOCK/artifacts" sub2api
sha256sum "$MOCK/artifacts/package.tar.gz" | sed "s#  .*#  sub2api_1.2.3-personal.2_linux_amd64.tar.gz#" >"$MOCK/artifacts/checksums.txt"
printf '#!/bin/sh\nold\n' >"$TMP/old-sub2api"; chmod +x "$TMP/old-sub2api"
export MOCK_OLD_EXE="$TMP/old-sub2api" DROPIN_PATH="$TMP/dropin/90-managed-release.conf"
printf '%s\n' "$MOCK_OLD_EXE" >"$MOCK_SYSTEMCTL_EXE"
echo 124 >"$MOCK_SYSTEMCTL_NEXT_PID"

expect_fail() { if "$@"; then echo "expected failure: $*" >&2; exit 1; fi; }
reset_run() {
  rm -rf -- "$TMP/deployed" "$TMP/state/transactions" "$TMP/dropin"
  mkdir -p -- "$TMP/deployed" "$TMP/state"
  echo 123 >"$MOCK_SYSTEMCTL_STATE"
  printf '%s\n' "$MOCK_OLD_EXE" >"$MOCK_SYSTEMCTL_EXE"
  echo 124 >"$MOCK_SYSTEMCTL_NEXT_PID"
  unset MIGRATION_FAIL HEALTH_FAIL STOP_FAIL ATOMIC_COPY_FAIL
}
sed "s/loLollipop\\/Sub2api/wrong\\/repo/" "$SUB2API_UPDATER_CONFIG" >"$TMP/bad-config"
chmod 0600 "$TMP/bad-config"
expect_fail env SUB2API_UPDATER_CONFIG="$TMP/bad-config" "$UPDATER" latest
cp "$MOCK/artifacts/checksums.txt" "$MOCK/artifacts/checksums.good"; printf '%064d  sub2api_1.2.3-personal.2_linux_amd64.tar.gz\n' 0 >"$MOCK/artifacts/checksums.txt"; expect_fail "$UPDATER" latest; cp "$MOCK/artifacts/checksums.good" "$MOCK/artifacts/checksums.txt"
reset_run
mkdir -p -- "$TMP/malicious"
ln -sf /etc/passwd "$TMP/malicious/sub2api"
tar -czf "$MOCK/artifacts/package.tar.gz" -C "$TMP/malicious" sub2api
sha256sum "$MOCK/artifacts/package.tar.gz" | sed "s#  .*#  sub2api_1.2.3-personal.2_linux_amd64.tar.gz#" >"$MOCK/artifacts/checksums.txt"
expect_fail "$UPDATER" latest; [[ $(<"$MOCK_SYSTEMCTL_STATE") == 123 ]] || exit 1
tar -czf "$MOCK/artifacts/package.tar.gz" -C "$MOCK/artifacts" sub2api
sha256sum "$MOCK/artifacts/package.tar.gz" | sed "s#  .*#  sub2api_1.2.3-personal.2_linux_amd64.tar.gz#" >"$MOCK/artifacts/checksums.txt"
reset_run; export MIGRATION_FAIL=1; expect_fail "$UPDATER" latest; [[ $(<"$MOCK_SYSTEMCTL_STATE") == 123 ]] || exit 1; unset MIGRATION_FAIL
reset_run; mkdir -p -- "$(dirname -- "$DROPIN_PATH")"; printf '[Service]\nExecStart=%s\n' "$MOCK_OLD_EXE" >"$DROPIN_PATH"; export HEALTH_FAIL=1; expect_fail "$UPDATER" latest
grep -q -- "$MOCK_OLD_EXE" "$DROPIN_PATH"
[[ $(<"$MOCK_SYSTEMCTL_EXE") == "$MOCK_OLD_EXE" ]] || { echo 'rollback did not restore old executable' >&2; exit 1; }
running_exe=$(<"$MOCK_SYSTEMCTL_EXE")
[[ $(sha256sum "$running_exe" | awk '{print $1}') == $(sha256sum "$MOCK_OLD_EXE" | awk '{print $1}') ]] || { echo 'rollback restored wrong executable hash' >&2; exit 1; }
[[ $(<"$MOCK_SYSTEMCTL_STATE") != 0 ]] || { echo 'rollback left service stopped' >&2; exit 1; }
unset HEALTH_FAIL
reset_run; mkdir -p -- "$(dirname -- "$DROPIN_PATH")"; printf '[Service]\nExecStart=%s\n' "$MOCK_OLD_EXE" >"$DROPIN_PATH"
export ATOMIC_COPY_FAIL=1 HEALTH_FAIL=1
expect_fail "$UPDATER" latest
grep -q 'ExecStart=.*/sub2api' "$DROPIN_PATH" || { echo 'failed atomic copy replaced the active drop-in' >&2; exit 1; }
! grep -q 'partial-copy' "$DROPIN_PATH" || { echo 'failed atomic copy published partial content' >&2; exit 1; }
[[ -z $(find "$(dirname -- "$DROPIN_PATH")" -maxdepth 1 -name '.90-managed-release.conf.*' -print -quit) ]] || { echo 'failed atomic copy left a temporary drop-in' >&2; exit 1; }
unset ATOMIC_COPY_FAIL HEALTH_FAIL
reset_run; "$UPDATER" latest; grep -q 'ExecStart=.*/sub2api' "$DROPIN_PATH"; test -s "$TMP/state/last-good"
[[ -z $(find "$TMP/deployed" -maxdepth 1 -name '*.staging.*' -print -quit) ]] || { echo 'uncommitted staging directory remains' >&2; exit 1; }
# Reusing an already committed immutable directory must remain idempotent.
"$UPDATER" latest
echo 'offline vps release updater tests passed'
