#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_root=$(cd "${TMPDIR:-/tmp}" && pwd)
test_dir=$(mktemp -d "$test_root/sub2api-install-XXXXXXXX")
trap '[[ "$test_dir" == "$test_root"/sub2api-install-* ]] && rm -rf -- "$test_dir"' EXIT
mkdir -p "$test_dir/bin" "$test_dir/archive"
printf 'verified binary\n' > "$test_dir/archive/sub2api"
archive_name=sub2api_9.9.9_linux_amd64.tar.gz
tar -czf "$test_dir/$archive_name" -C "$test_dir/archive" sub2api
sha256sum "$test_dir/$archive_name" > "$test_dir/checksums.txt"
# The checksums file must contain a basename, as real release assets do.
checksum=$(sha256sum "$test_dir/$archive_name" | awk '{print $1}')
printf '%s  %s\n' "$checksum" "$archive_name" > "$test_dir/checksums.txt"

cat > "$test_dir/bin/curl" <<'EOF'
#!/bin/bash
set -euo pipefail
test "$1" = '-fsSL'
url=$2
test "$3" = '-o'
if [[ "$url" == */checksums.txt ]]; then
    case "$TEST_CASE" in
        unavailable) exit 22 ;;
        missing) printf '%s  other.tar.gz\n' "${TEST_CHECKSUM}" > "$4" ;;
        mismatch) printf '%064d  %s\n' 0 "$TEST_ARCHIVE" > "$4" ;;
        malformed) printf 'not-a-checksum  %s\n' "$TEST_ARCHIVE" > "$4" ;;
        success) cp "$TEST_FIXTURES/checksums.txt" "$4" ;;
        binary-entry) printf '%s *%s\n' "$TEST_CHECKSUM" "$TEST_ARCHIVE" > "$4" ;;
        *) exit 1 ;;
    esac
else
    test "$TEST_CASE" != archive-unavailable || exit 22
    cp "$TEST_FIXTURES/$TEST_ARCHIVE" "$4"
fi
EOF
chmod +x "$test_dir/bin/curl"

for scenario in unavailable missing mismatch malformed archive-unavailable success binary-entry; do
    target="$test_dir/install-$scenario"
    mkdir "$target"
    printf 'previous binary\n' > "$target/sub2api"
    status=0
    TEST_CASE="$scenario" TEST_FIXTURES="$test_dir" TEST_CHECKSUM="$checksum" TEST_ARCHIVE="$archive_name" \
        PATH="$test_dir/bin:$PATH" bash -c '
            source <(head -n -1 "$1")
            LATEST_VERSION=v9.9.9 OS=linux ARCH=amd64 INSTALL_DIR="$2"
            download_and_extract
        ' bash "$ROOT_DIR/deploy/install.sh" "$target" > "$test_dir/$scenario.log" 2>&1 || status=$?
    case "$scenario" in
        success|binary-entry)
            test "$status" -eq 0
            cmp "$target/sub2api" "$test_dir/archive/sub2api"
            ;;
        *)
            test "$status" -ne 0
            test "$(cat "$target/sub2api")" = 'previous binary'
            ;;
    esac
done
echo 'install checksum checks passed (7 cases)'
