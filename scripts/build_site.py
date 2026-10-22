"""Embed GitHub release snapshots so visitors do not need GitHub API access."""

import html
import json
import re
import sys
from pathlib import Path


def build(site, latest, releases):
    if not latest.get("tag_name") or latest.get("draft") or latest.get("prerelease"):
        raise ValueError("Expected a published stable latest release")
    if not isinstance(releases, list):
        raise ValueError("Expected a release list")
    snapshot = json.dumps({"latest": latest, "releases": releases}, ensure_ascii=False)
    # Release notes are untrusted text, including inside a JSON script element.
    snapshot = snapshot.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")
    embedded = f'<script id="release-data" type="application/json">{snapshot}</script>\n  '
    tag = html.escape(latest["tag_name"])
    summary = re.sub(r"^NEMCBridge\s+v\S+\s*[·:：|—-]?\s*", "", latest.get("name", ""))
    label = f"{tag} 已发布 · {html.escape(summary or '查看更新内容')}"
    for path in (site / "index.html", site / "docs.html"):
        source = path.read_text(encoding="utf-8")
        source = re.sub(r'<script id="release-data" type="application/json">.*?</script>\s*', "", source, flags=re.S)
        source = source.replace('<script src="main.js" defer>', embedded + '<script src="main.js" defer>')
        source = re.sub(r'(?<=data-release-label>)[^<]*', lambda _: label, source)
        source = re.sub(r'(?<=data-version>)[^<]*', lambda _: tag, source)
        source = re.sub(r'(?<=data-version-label>)[^<]*', lambda _: f"{tag} · 免安装单文件", source)
        path.write_text(source, encoding="utf-8")


if __name__ == "__main__":
    build(Path(sys.argv[1]), json.loads(Path(sys.argv[2]).read_text(encoding="utf-8")),
          json.loads(Path(sys.argv[3]).read_text(encoding="utf-8")))
