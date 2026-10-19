"""Tests use generated, deliberately synthetic credential-shaped values."""

from __future__ import annotations

import base64
from contextlib import redirect_stderr, redirect_stdout
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


SCRIPT = Path(__file__).resolve().parents[1] / "check_sensitive.py"
SPEC = importlib.util.spec_from_file_location("check_sensitive", SCRIPT)
assert SPEC and SPEC.loader
guard = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(guard)


def token() -> str:
    return "sk-" + "Ab9Cd8Ef7Gh6Ij5Kl4Mn3Op2Qr1St0Uv" * 2


def jwt() -> str:
    def encoded(value: dict[str, str]) -> str:
        return base64.urlsafe_b64encode(json.dumps(value).encode()).decode().rstrip("=")
    return ".".join((encoded({"alg": "HS256", "typ": "JWT"}),
                     encoded({"sub": "synthetic-testing-subject"}), "QWJjZGVmZ2hpamtsbW5vcA"))


class DetectionTests(unittest.TestCase):
    def test_no_directory_or_extension_exemptions(self) -> None:
        for path in ("src/client.go", "src/client_test.go", "testdata/token.json",
                     "fixtures/client.txt", "frontend/__tests__/client.spec.ts",
                     "changes.patch", "assets/icon.svg", "data.bin"):
            with self.subTest(path=path):
                findings = guard.scan(path, ("safe\n" + token()).encode())
                self.assertEqual([(path, 2, "provider API token")], findings)

    def test_provider_shapes(self) -> None:
        for value in ("sk-ant-api03-" + "Ab12Cd34" * 8,
                      "sk-proj-" + "Ab12Cd34" * 8,
                      "ghp_" + "Ab12Cd34" * 5,
                      "github_pat_" + "Ab12Cd34" * 8,
                      "AKIA" + "AB12CD34EF56GH78",
                      "ASIA" + "AB12CD34EF56GH78",
                      "AIza" + "Ab12Cd3" * 5,
                      "xoxb-" + "1234567890-" * 3):
            with self.subTest(prefix=value.split("-")[0][:4]):
                self.assertTrue(guard.scan("fixture.txt", value.encode()))

    def test_explicit_placeholders(self) -> None:
        for value in ("sk-" + "x" * 40, "sk-proj-" + "0" * 40,
                      "sk-proj-your-api-key-placeholder", "sk-ant-test-api-token-not-real",
                      "ghp_" + "x" * 40, "AKIA" + "X" * 16):
            with self.subTest(value=value):
                self.assertEqual([], guard.scan("README.md", value.encode()))

    def test_placeholder_substring_does_not_exempt_secret(self) -> None:
        for marker in ("example", "test", "demo", "fake", "xxxx"):
            value = token() + marker
            with self.subTest(marker=marker):
                self.assertTrue(guard.scan("src/app_test.go", value.encode()))

    def test_jwt_and_pem(self) -> None:
        header = "-----BEGIN " + "RSA PRIVATE KEY-----"
        findings = guard.scan("fixtures/keys.patch", (jwt() + "\n" + header).encode())
        self.assertEqual([("fixtures/keys.patch", 1, "JWT"),
                          ("fixtures/keys.patch", 2, "PEM private key")], findings)

    def test_url_credentials(self) -> None:
        for scheme, authority in (("https", "relay.service.net/path"),
                                  ("postgresql", "db.service.net/db"),
                                  ("postgresql", "10.0.0.5/db"),
                                  ("https", "api.openai.com")):
            password = "p%40ss%3Aword" if authority == "api.openai.com" else "synthetic-secret"
            value = scheme + "://operator:" + password + "@" + authority
            self.assertEqual([("config.yml", 1, "URL credentials")],
                             guard.scan("config.yml", value.encode()))

    def test_standard_vendors_and_documentation_addresses(self) -> None:
        for value in ("https://api.openai.com/v1", "https://api.anthropic.com",
                      "https://generativelanguage.googleapis.com", "https://api.x.ai",
                      "https://public-vendor.net/v1", "https://1.1.1.1/dns-query",
                      "https://user:pass@192.0.2.1", "https://user:pass@198.51.100.2",
                      "https://user:pass@203.0.113.3", "https://user:pass@[2001:db8::1]",
                      "https://user:pass@relay.example.com",
                      "postgres://user:password@db.vendor.net/db",
                      "https://user:${API_PASSWORD}@api.vendor.net"):
            with self.subTest(value=value):
                self.assertEqual([], guard.scan("docs/config.md", value.encode()))

    def test_utf16_and_utf32_content(self) -> None:
        for encoding in ("utf-16", "utf-32"):
            self.assertTrue(guard.scan("asset.bin", token().encode(encoding)))

    def test_reviewed_fixture_is_bound_to_exact_path_and_line(self) -> None:
        path = "backend/internal/service/audit_log_test.go"
        text = (SCRIPT.parent.parent / path).read_text(encoding="utf-8")
        self.assertEqual([], guard.scan(path, text.encode()))
        self.assertTrue(guard.scan("new_fixture_test.go", text.encode()))
        lines = text.splitlines()
        number = next(i for i, line in enumerate(lines) if '"long"' in line)
        # A credential added to a previously reviewed line must be caught.
        lines[number] += " " + token()
        self.assertTrue(guard.scan(path, "\n".join(lines).encode()))

    def test_report_escapes_path_controls(self) -> None:
        self.assertEqual(r"path\nname:2: JWT", guard.location("path\nname", 2, "JWT"))


class FailureTests(unittest.TestCase):
    def test_git_command_failure_does_not_print_stderr(self) -> None:
        result = subprocess.CompletedProcess([], 1, stdout=b"", stderr=token().encode())
        with patch.object(guard.subprocess, "run", return_value=result):
            output, errors = io.StringIO(), io.StringIO()
            with redirect_stdout(output), redirect_stderr(errors):
                self.assertEqual(2, guard.main(["--all"]))
        self.assertNotIn(token(), output.getvalue() + errors.getvalue())
        self.assertIn("Git command failed", errors.getvalue())

    def test_missing_git_fails_closed(self) -> None:
        with patch.object(guard.subprocess, "run", side_effect=FileNotFoundError):
            with self.assertRaises(guard.ScanError):
                guard.git(["ls-files"], Path.cwd())

    def test_file_read_failure_fails_closed(self) -> None:
        with patch.object(Path, "read_bytes", side_effect=PermissionError):
            with self.assertRaises(guard.ScanError):
                guard.read_content(Path.cwd(), "not-readable.txt", staged=False)

    def test_index_read_failure_fails_closed(self) -> None:
        with patch.object(guard, "git", side_effect=guard.ScanError("Git command failed")):
            with self.assertRaises(guard.ScanError) as caught:
                guard.read_content(Path.cwd(), "broken.txt", staged=True)
        self.assertEqual("index read failed", caught.exception.kind)


class GitIntegrationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.run_git("init", "-q")

    def run_git(self, *args: str) -> None:
        subprocess.run(["git", *args], cwd=self.root, check=True,
                       stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def run_guard(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run([os.sys.executable, str(SCRIPT), *args], cwd=self.root,
                              capture_output=True, text=True, check=False)

    def test_staged_reads_index_and_report_is_redacted(self) -> None:
        path = self.root / "fixture.svg"
        path.write_text("\n" + token(), encoding="utf-8")
        self.run_git("add", "--", path.name)
        path.write_text("safe worktree", encoding="utf-8")
        result = self.run_guard("--staged")
        self.assertEqual(1, result.returncode)
        self.assertIn("fixture.svg:2: provider API token", result.stdout)
        self.assertNotIn(token(), result.stdout + result.stderr)
        self.assertNotIn(token()[3:20], result.stdout + result.stderr)
        self.assertEqual(0, self.run_guard("--all").returncode)

    def test_staged_ignores_unstaged_secret(self) -> None:
        path = self.root / "app_test.go"
        path.write_text("safe staged content", encoding="utf-8")
        self.run_git("add", "--", path.name)
        path.write_text(token(), encoding="utf-8")
        self.assertEqual(0, self.run_guard("--staged").returncode)
        self.assertEqual(1, self.run_guard("--all").returncode)

    def test_tracked_file_missing_fails_closed(self) -> None:
        path = self.root / "deleted.txt"
        path.write_text("safe", encoding="utf-8")
        self.run_git("add", "--", path.name)
        path.unlink()
        result = self.run_guard("--all")
        self.assertEqual(2, result.returncode)
        self.assertIn("deleted.txt:0: file read failed", result.stderr)
        self.assertEqual(0, self.run_guard("--staged").returncode)

    def test_outside_git_repository_fails_closed(self) -> None:
        with tempfile.TemporaryDirectory() as outside:
            result = subprocess.run([os.sys.executable, str(SCRIPT), "--all"],
                                    cwd=outside, capture_output=True, text=True, check=False)
        self.assertEqual(2, result.returncode)
        self.assertIn("Git command failed", result.stderr)


if __name__ == "__main__":
    unittest.main()
