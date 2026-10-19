#!/usr/bin/env python3
"""Scan tracked worktree files or staged Git blobs for recognizable credentials.

Uses only the Python standard library. Findings contain locations and types,
never matched values. No file extensions or fixture directories are exempt.
"""

from __future__ import annotations

import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit


SECRET_PATTERNS = (
    ("provider API token", re.compile(
        r"(?<![\w-])sk-(?:(?:ant-(?:api\d+-)?|proj-|svcacct-)[A-Za-z0-9_-]{20,}"
        r"|[A-Za-z0-9]{32,})(?![\w-])")),
    ("GitHub token", re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{50,})\b")),
    ("AWS access key", re.compile(r"\b(?:AKIA|ASIA)[A-Z0-9]{16}\b")),
    ("Google API key", re.compile(r"\bAIza[A-Za-z0-9_-]{35}\b")),
    ("Slack token", re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{20,}\b")),
    ("JWT", re.compile(r"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b")),
    ("PEM private key", re.compile(r"-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----")),
)
URL_RE = re.compile(r"\b[a-zA-Z][a-zA-Z0-9+.-]{0,31}://[^\s\"'<>`]+")
# These are complete, explicitly fake token bodies, not substrings to ignore.
PLACEHOLDER_BODY = re.compile(
    r"(?:[xX]{8,}|0{8,}|(?:your|test|example|dummy|fake|demo|placeholder)"
    r"(?:[-_](?:api|key|token|secret|placeholder|only|value|here|not|real|valid|for|testing|[xX]+))+)",
    re.IGNORECASE,
)


# Reviewed synthetic fixture lines only. Any byte change on the line requires
# review again; new credentials on an existing fixture line are still detected.
REVIEWED_SYNTHETIC_LINES = {
    # Truncated PEM and literal data exercise formatting only.
    ('backend/internal/payment/provider/wxpay_test.go', 'PEM private key',
     '2e16e2c4292adfa412ef99cd6453eb8f2ae0f70cbb2065fcda199665bf6d69b2'),
    # Truncated PEM and literal data exercise formatting only.
    ('backend/internal/payment/provider/wxpay_test.go', 'PEM private key',
     '58a1b3cd84e42ee226fe36d20933eff77cb6b80512dbe48453105173d5b8e6ea'),
    # Truncated PEM and literal data exercise formatting only.
    ('backend/internal/payment/provider/wxpay_test.go', 'PEM private key',
     'c32a5cec48950de24f890a0813d43f013fd06628d548d52671e26ba41246c7ca'),
    # Truncated PEM and literal data exercise formatting only.
    ('backend/internal/payment/provider/wxpay_test.go', 'PEM private key',
     '1df05b79372f538b2656c913a7c151e1310c9788a68d2e0d4f95488d1adbd247'),
    # Synthetic URL parser fixture; no external network call.
    ('backend/internal/pkg/proxyurl/parse_test.go', 'URL credentials',
     '860788753facba912230cb4b12bd24f1e98ee3ee11fd3d68ebc17a9743a5c35d'),
    # Deliberately tampered JWT in authentication rejection test.
    ('backend/internal/server/middleware/jwt_auth_test.go', 'JWT',
     'f1c9a4506976c5f7b37e6c088c122d844b6b08b55495a83b0919e8bd57ac9887'),
    # Synthetic sequential token tests masking boundaries.
    ('backend/internal/service/audit_log_test.go', 'provider API token',
     '329e574651d305ec1906b14e4ea9dd7d1204512d3ccdcb3c5126484be3f00983'),
    # Invalid PEM marker tests redaction and validation.
    ('backend/internal/service/batch_image_provider_vertex_test.go', 'PEM private key',
     'a58b38f4c9a34a6dacb9942b5189d83df925c1168cd2a9486ae792e57c9b2e9d'),
    # Invalid PEM marker tests redaction and validation.
    ('backend/internal/service/batch_image_provider_vertex_test.go', 'PEM private key',
     'd94bbc07e470a0ad0fff2ea6406e2e3b9a571d37ba74eddcce9788f3e8d54d90'),
    # Synthetic JWT exercises log redaction.
    ('backend/internal/service/content_moderation_test.go', 'JWT',
     '3061e06d795ba0f567486861325abf86c322c9560bb9e8e3e7940a404b873d43'),
    # Synthetic malformed proxy credentials test rejection.
    ('backend/internal/service/setting_codex_ticket_test.go', 'URL credentials',
     'a2908a70751297effb0ede88b603eb08da4a89efda4443f7fd9dd787bcd88d54'),
    # Invalid abc PEM fixture; no usable key.
    ('backend/internal/service/upstream_models_vertex_test.go', 'PEM private key',
     '19492e81af1d4cee89e2f101a2db5475a6880b025f78054507aeca6f6ebf13fd'),
    # Invalid abc PEM fixture; no usable key.
    ('backend/internal/service/vertex_service_account_test.go', 'PEM private key',
     '4c423343e46425e33a15dcefb704a6d16b90ba34a1cca2fcf9f9065794dcb879'),
    # Truncated MIIE PEM in service-account form fixture.
    ('frontend/src/components/account/__tests__/EditAccountModal.spec.ts', 'PEM private key',
     '7b1f6cf02bb3e2081f535771aa8522f76d04e78c376b4a00af8a76fd32b76f70'),
    # Frozen test-only internal-error redaction patch fixture.
    ('openspec/changes/add-openai-compatible-prompt-audit/source-freeze/aicodex-prompt-audit-tracked.patch', 'URL credentials',
     'e7b5e2d05e7428620103cca501be5a047fc41265f00ef8bf58936d5d710d206c'),
}


class ScanError(Exception):
    """An operational failure with a deliberately value-free diagnostic."""

    def __init__(self, kind: str, path: str = "<git>") -> None:
        super().__init__(kind)
        self.kind = kind
        self.path = path


def is_placeholder_token(value: str) -> bool:
    if value.startswith("sk-"):
        body = re.sub(r"^(?:ant-(?:api\d+-)?|proj-|svcacct-)", "", value[3:])
    elif value.startswith(("AKIA", "ASIA", "AIza")):
        body = value[4:]
    elif value.startswith(("ghp_", "gho_", "ghu_", "ghs_", "ghr_")):
        body = value[4:]
    elif value.startswith("github_pat_"):
        body = value[11:]
    elif re.match(r"xox[baprs]-", value):
        body = value[5:]
    else:
        return False
    return PLACEHOLDER_BODY.fullmatch(body) is not None


def is_documentation_host(host: str) -> bool:
    host = host.lower().rstrip(".")
    if host == "localhost" or host.endswith(".localhost"):
        return True
    if any(host == suffix or host.endswith("." + suffix)
           for suffix in ("example.com", "example.net", "example.org", "invalid", "test", "example")):
        return True
    try:
        address = ipaddress.ip_address(host)
    except ValueError:
        return False
    # Private networks can contain production services; only local and reserved
    # documentation addresses are examples by definition.
    documentation_networks = ("192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32")
    return (address.is_loopback or address.is_unspecified
            or any(address in ipaddress.ip_network(network) for network in documentation_networks))


def contains_url_credentials(value: str) -> bool:
    try:
        url = urlsplit(value)
        if not url.hostname or url.username is None or url.password is None:
            return False
        if is_documentation_host(url.hostname):
            return False
        username, password = unquote(url.username), unquote(url.password)
    except ValueError:
        return False
    if not username or not password:
        return False
    # Exact template syntax and explicit example pairs are safe to publish.
    if re.fullmatch(r"\$\{[A-Za-z_][A-Za-z0-9_]*\}|\{\{[^{}]+\}\}", password):
        return False
    return (username.lower(), password.lower()) not in {
        ("user", "password"), ("username", "password"), ("user", "pass"),
    }


def decode_content(content: bytes) -> str:
    if content.startswith((b"\xff\xfe\x00\x00", b"\x00\x00\xfe\xff")):
        return content.decode("utf-32", errors="replace")
    if content.startswith((b"\xff\xfe", b"\xfe\xff")):
        return content.decode("utf-16", errors="replace")
    return content.decode("utf-8", errors="replace")


def scan(path: str, content: bytes) -> list[tuple[str, int, str]]:
    findings: list[tuple[str, int, str]] = []
    lines = decode_content(content).splitlines()
    for number, line in enumerate(lines, 1):
        for kind, pattern in SECRET_PATTERNS:
            if any(not is_placeholder_token(match.group()) for match in pattern.finditer(line)):
                findings.append((path, number, kind))
        if any(contains_url_credentials(match.group()) for match in URL_RE.finditer(line)):
            findings.append((path, number, "URL credentials"))
    return [(file, number, kind) for file, number, kind in findings
            if (file, kind, hashlib.sha256(lines[number-1].encode()).hexdigest())
            not in REVIEWED_SYNTHETIC_LINES]


def git(args: list[str], cwd: Path) -> bytes:
    try:
        result = subprocess.run(["git", *args], cwd=cwd, capture_output=True, check=False)
    except OSError as error:
        raise ScanError("Git execution failed") from error
    if result.returncode:
        raise ScanError("Git command failed")
    return result.stdout


def collect_files(root: Path, staged: bool) -> list[str]:
    args = (["diff", "--cached", "--name-only", "--diff-filter=ACMRTU", "-z"]
            if staged else ["ls-files", "--cached", "-z"])
    return list(dict.fromkeys(os.fsdecode(path) for path in git(args, root).split(b"\0") if path))


def read_content(root: Path, path: str, staged: bool) -> bytes:
    if staged:
        try:
            return git(["cat-file", "blob", ":" + path], root)
        except ScanError as error:
            raise ScanError("index read failed", path) from error
    try:
        file = root / path
        if file.is_symlink():
            return os.fsencode(os.readlink(file))
        return file.read_bytes()
    except OSError as error:
        raise ScanError("file read failed", path) from error


def location(path: str, line: int, kind: str) -> str:
    # Escape control characters in filenames to keep reports to one safe line.
    safe_path = json.dumps(path, ensure_ascii=True)[1:-1]
    return f"{safe_path}:{line}: {kind}"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--all", action="store_true", help="scan all tracked worktree files")
    mode.add_argument("--staged", action="store_true", help="scan staged Git blobs (default)")
    args = parser.parse_args(argv)
    try:
        root = Path(os.fsdecode(git(["rev-parse", "--show-toplevel"], Path.cwd()).rstrip(b"\r\n")))
        files = collect_files(root, staged=not args.all)
        findings = []
        for path in files:
            findings.extend(scan(path, read_content(root, path, staged=not args.all)))
    except ScanError as error:
        print(location(error.path, 0, error.kind), file=sys.stderr)
        return 2
    for finding in findings:
        print(location(*finding))
    if findings:
        return 1
    print(f"check_sensitive: OK ({len(files)} files)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
