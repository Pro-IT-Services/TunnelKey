"""Builds the public web pages served by GitHub Pages from docs/.

    python docs/build_pages.py

docs/privacy-policy.md -> docs/privacy-policy.html (the URL used in the app and
on Google Play), plus docs/index.html. Requires: pip install markdown
"""

from pathlib import Path

import markdown

DOCS = Path(__file__).resolve().parent

TEMPLATE = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title}</title>
<style>
  :root {{ --bg:#0b0f14; --raised:#131a22; --line:#2a3542; --ink:#e6edf3; --muted:#93a1b0; --brass:#e3a93b; }}
  @media (prefers-color-scheme: light) {{
    :root {{ --bg:#f5f3ee; --raised:#fff; --line:#d8d3c8; --ink:#161a1f; --muted:#55606b; --brass:#8a5a00; }}
  }}
  * {{ box-sizing: border-box; }}
  body {{ margin:0; background:var(--bg); color:var(--ink);
         font:16px/1.65 -apple-system, "Segoe UI", Roboto, system-ui, sans-serif; }}
  main {{ max-width: 760px; margin: 0 auto; padding: 48px 20px 80px; }}
  h1 {{ font-size: clamp(28px, 5vw, 38px); line-height: 1.15; margin: 0 0 16px; }}
  h2 {{ font-size: 21px; margin: 40px 0 10px; }}
  a {{ color: var(--brass); }}
  p, li {{ color: var(--ink); }}
  table {{ width: 100%; border-collapse: collapse; margin: 12px 0; font-size: 15px; display: block; overflow-x: auto; }}
  th, td {{ text-align: left; padding: 10px 12px; border-bottom: 1px solid var(--line); vertical-align: top; }}
  th {{ color: var(--muted); font-weight: 600; font-size: 13px; text-transform: uppercase; letter-spacing: .06em; }}
  code {{ font-family: ui-monospace, Consolas, monospace; font-size: .92em; }}
  .brand {{ display:flex; align-items:center; gap:10px; margin-bottom: 32px; color: var(--muted); font-weight:600; }}
  .brand svg {{ width: 32px; height: 32px; }}
</style>
</head>
<body>
<main>
<div class="brand">
  <svg viewBox="0 0 48 48" aria-hidden="true"><rect width="48" height="48" rx="11" fill="#0B0F14"/><path d="M11 41V23a13 13 0 0 1 26 0v18" fill="none" stroke="#E3A93B" stroke-width="4.5" stroke-linecap="round"/><path d="M17.5 41V23.5a6.5 6.5 0 0 1 13 0V41" fill="none" stroke="#5A6B7D" stroke-width="1.8" stroke-linecap="round"/><path d="M24 21.5a3 3 0 0 1 1.6 5.5l.9 5.5h-5l.9-5.5a3 3 0 0 1 1.6-5.5z" fill="#F5F3EE"/></svg>
  Tunnelkey
</div>
{body}
</main>
</body>
</html>
"""

INDEX = """# Tunnelkey

Open-source VPN client for OpenVPN servers that sign in with a password and an
authenticator code — for Android, iOS, Windows, macOS and Linux — with a
self-hosted provisioning server for organisations.

- [Create setup codes and desktop setup files](provision/) — runs entirely in your browser, nothing is uploaded
- [Desktop downloads](https://github.com/Pro-IT-Services/TunnelKey/releases)
- [Privacy policy](privacy-policy.html)
- [Source code on GitHub](https://github.com/Pro-IT-Services/TunnelKey)
- Contact: [develop@pro-it.sk](mailto:develop@pro-it.sk)

Published by ProIT services.
"""


def render(md_text: str, title: str, out: Path) -> None:
    body = markdown.markdown(md_text, extensions=["tables"])
    out.write_text(TEMPLATE.format(title=title, body=body), encoding="utf-8")
    print("wrote", out.relative_to(DOCS.parent))


def main() -> None:
    render((DOCS / "privacy-policy.md").read_text(encoding="utf-8"), "Tunnelkey Privacy Policy", DOCS / "privacy-policy.html")
    render(INDEX, "Tunnelkey", DOCS / "index.html")
    # Serve the HTML as-is; don't let Jekyll process the docs folder.
    (DOCS / ".nojekyll").write_text("", encoding="utf-8")


if __name__ == "__main__":
    main()
