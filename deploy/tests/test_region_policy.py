"""Offline checks for the public notice and its nginx integration contract.

Run with: python -m unittest discover -s deploy/tests -p test_region_policy.py
The deployed nginx configuration and real client-IP handling need an integration
check on the server; these checks intentionally need no third-party packages.
"""

from html.parser import HTMLParser
from pathlib import Path
import re
import unittest


POLICY_DIR = Path(__file__).resolve().parents[1] / "nginx" / "region-policy"
OFFICIAL_URL = "https://www.cac.gov.cn/2023-07/13/c_1690898327029107.htm"


class NoticeParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.elements: list[tuple[str, dict[str, str | None]]] = []
        self.visible_text: list[str] = []
        self.styles: list[str] = []
        self.in_style = False

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        self.elements.append((tag, dict(attrs)))
        if tag == "style":
            self.in_style = True

    def handle_endtag(self, tag: str) -> None:
        if tag == "style":
            self.in_style = False

    def handle_data(self, data: str) -> None:
        if self.in_style:
            self.styles.append(data)
        else:
            self.visible_text.append(data)


class RegionPolicyTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.html = (POLICY_DIR / "__region_unavailable.html").read_text(encoding="utf-8")
        cls.server = (POLICY_DIR / "server.conf").read_text(encoding="utf-8")
        cls.gate = (POLICY_DIR / "web-gate.conf").read_text(encoding="utf-8")
        cls.page = NoticeParser()
        cls.page.feed(cls.html)

    def test_bilingual_notice_has_accessible_structure(self) -> None:
        elements = self.page.elements
        self.assertIn(("html", {"lang": "zh-CN"}), elements)
        self.assertEqual(sum(tag == "main" for tag, _ in elements), 1)
        self.assertEqual(sum(tag == "h1" for tag, _ in elements), 1)
        ids = {attrs["id"] for _, attrs in elements if "id" in attrs}
        for _, attrs in elements:
            if "aria-labelledby" in attrs:
                for target in attrs["aria-labelledby"].split():
                    self.assertIn(target, ids)
        self.assertTrue(any(attrs.get("lang") == "en" for _, attrs in elements))
        self.assertTrue(any(tag == "meta" and attrs.get("name") == "viewport" for tag, attrs in elements))
        text = "".join(self.page.visible_text)
        self.assertIn("本网站暂不向中国大陆地区开放", text)
        self.assertIn("This website is currently unavailable in mainland China.", text)

    def test_official_reference_is_the_only_link(self) -> None:
        links = [attrs for tag, attrs in self.page.elements if tag == "a"]
        self.assertEqual([attrs.get("href") for attrs in links], [OFFICIAL_URL])
        self.assertIn("noopener", links[0]["rel"])
        self.assertIn("noreferrer", links[0]["rel"])
        text = "".join(self.page.visible_text)
        self.assertIn("《生成式人工智能服务管理暂行办法》", text)
        self.assertIn(("time", {"datetime": "2023-08-15"}), self.page.elements)

    def test_no_javascript_or_external_resources(self) -> None:
        for tag, attrs in self.page.elements:
            self.assertNotIn(tag, {"script", "iframe", "object", "embed", "link", "img", "form"})
            for name in attrs:
                self.assertFalse(name.startswith("on"), name)
                self.assertNotIn(name, {"src", "srcset", "action", "formaction", "style"})
            if "href" in attrs:
                self.assertEqual(attrs["href"], OFFICIAL_URL)
        styles = "".join(self.page.styles)
        self.assertNotRegex(styles, r"(?i)@import|url\s*\(")
        self.assertIn("@media (max-width: 600px)", styles)
        self.assertIn(":focus-visible", styles)

    def test_public_copy_does_not_expose_routing_or_claim_a_legal_order(self) -> None:
        text = "".join(self.page.visible_text)
        self.assertNotRegex(text, r"(?i)\bapi\b|\bkey\b|/v1/|nginx|cloudflare|接口|密钥|例外|豁免|绕过")
        for phrase in ("依法关闭", "强制关闭", "法院", "court order", "required by law"):
            self.assertNotIn(phrase, text.lower())

    def test_nginx_keeps_451_and_uses_the_packaged_internal_file(self) -> None:
        server = re.sub(r"#.*", "", self.server)
        gate = re.sub(r"#.*", "", self.gate)
        self.assertRegex(server, r"error_page\s+451\s+/__region_unavailable\.html\s*;")
        self.assertNotRegex(server, r"error_page\s+451\s+=")
        self.assertRegex(server, r"location\s+=\s+/__region_unavailable\.html\s*\{")
        self.assertRegex(server, r"\binternal\s*;")
        self.assertRegex(server, r"root\s+/srv/www/sub2api-region-policy\s*;")
        self.assertRegex(server, r"default_type\s+text/html\s*;")
        self.assertRegex(server, r"charset\s+utf-8\s*;")
        self.assertRegex(gate, r"if\s*\(\$sub2api_is_china\)\s*\{\s*return\s+451\s*;\s*\}")
        self.assertNotRegex(server + gate, r"\bproxy_pass\b|\breturn\s+(?:200|301|302|444)\b")

    def test_error_response_has_cache_and_security_headers(self) -> None:
        headers = dict(re.findall(r'add_header\s+([\w-]+)\s+"([^"]*)"\s+always\s*;', self.server))
        self.assertIn("private", headers["Cache-Control"])
        self.assertIn("no-store", headers["Cache-Control"])
        self.assertEqual(headers["X-Content-Type-Options"], "nosniff")
        self.assertEqual(headers["X-Frame-Options"], "DENY")
        self.assertEqual(headers["Referrer-Policy"], "no-referrer")
        self.assertIn("max-age=", headers["Strict-Transport-Security"])
        self.assertIn("camera=()", headers["Permissions-Policy"])
        self.assertIn("noindex", headers["X-Robots-Tag"])
        csp = headers["Content-Security-Policy"]
        for directive in ("default-src 'none'", "style-src 'unsafe-inline'", "base-uri 'none'", "form-action 'none'", "frame-ancestors 'none'"):
            self.assertIn(directive, csp)
        self.assertNotIn("script-src 'unsafe-inline'", csp)


if __name__ == "__main__":
    unittest.main()
