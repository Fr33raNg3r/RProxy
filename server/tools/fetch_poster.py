#!/usr/bin/env python3
"""从 Wikimedia Commons 下载影片海报缩略图到 configs/site/posters/，并打印授权信息。

用法：PYTHONUTF8=1 python server/tools/fetch_poster.py 'slug=Commons 上的文件名' ...
只接受公有领域 / CC0 / CC BY（不含 SA、NC、ND）授权的图片，其他授权直接拒绝。
遇到 429 限流会自动退避重试。打印的制表符分隔行可追加到 configs/site/CREDITS.tsv。
本工具不随发布包分发（发布包只含 scripts/、configs/、install.sh）。
"""
import io
import json
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

UA = {"User-Agent": "RProxyPosterFetch/1.0 (https://github.com/Fr33raNg3r/RProxy; asset collection)"}
OUT = Path(__file__).resolve().parents[1] / "configs" / "site" / "posters"
ALLOWED = re.compile(r"^(public domain|cc0|pd|cc by \d(\.\d)?)$", re.I)


def get(url):
    for wait in (0, 20, 60, 120):
        time.sleep(wait)
        try:
            return urllib.request.urlopen(urllib.request.Request(url, headers=UA), timeout=60).read()
        except urllib.error.HTTPError as e:
            if e.code != 429:
                raise
    raise RuntimeError("持续被限流: " + url)


def main():
    # newline="\n"：Windows 下默认会把换行写成 CRLF，署名记录要求 LF
    sys.stdout.reconfigure(encoding="utf-8", newline="\n")
    OUT.mkdir(parents=True, exist_ok=True)
    for pair in sys.argv[1:]:
        slug, title = pair.split("=", 1)
        query = {
            "action": "query", "format": "json", "titles": "File:" + title, "prop": "imageinfo",
            "iiprop": "url|extmetadata", "iiextmetadatafilter": "LicenseShortName|Artist", "iiurlwidth": "500",
        }
        data = json.loads(get("https://commons.wikimedia.org/w/api.php?" + urllib.parse.urlencode(query)))
        page = next(iter(data["query"]["pages"].values()))
        if "imageinfo" not in page:
            print(f"{slug}: 在 Commons 上找不到 File:{title}，已跳过", file=sys.stderr)
            continue
        info = page["imageinfo"][0]
        meta = info["extmetadata"]
        lic = meta.get("LicenseShortName", {}).get("value", "").strip()
        if not ALLOWED.match(lic):
            print(f"{slug}: 授权 [{lic}] 不在允许范围内，已拒绝", file=sys.stderr)
            continue
        artist = re.sub(r"<[^>]+>", "", meta.get("Artist", {}).get("value", "")).strip()
        raw = get(info["thumburl"])
        if raw[:4] == b"\x89PNG":
            try:
                from PIL import Image
            except ImportError:
                print(f"{slug}: 缩略图是 PNG，需要 Pillow 才能转成 JPEG（pip install pillow），已跳过", file=sys.stderr)
                continue
            Image.open(io.BytesIO(raw)).convert("RGB").save(OUT / f"{slug}.jpg", "JPEG", quality=85)
        else:
            (OUT / f"{slug}.jpg").write_bytes(raw)
        url = "https://commons.wikimedia.org/wiki/File:" + urllib.parse.quote(title.replace(" ", "_"))
        print("\t".join([slug, title, lic, artist, url]), flush=True)
        time.sleep(5)


if __name__ == "__main__":
    main()
