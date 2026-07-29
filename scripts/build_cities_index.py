#!/usr/bin/env python3
"""Build data/cities_index.json.gz from dr5hn countries-states-cities-database.

Source (ODbL attribution required):
  https://github.com/dr5hn/countries-states-cities-database

Usage:
  python3 scripts/build_cities_index.py
  python3 scripts/build_cities_index.py /path/to/json-cities.json
"""

from __future__ import annotations

import gzip
import json
import os
import sys
import urllib.request

RELEASE_URL = (
    "https://github.com/dr5hn/countries-states-cities-database"
    "/releases/latest/download/json-cities.json.gz"
)

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
OUT = os.path.join(ROOT, "data", "cities_index.json.gz")


def load_cities(path: str | None) -> list[dict]:
    if path:
        opener = gzip.open if path.endswith(".gz") else open
        with opener(path, "rt", encoding="utf-8") as f:
            return json.load(f)
    print(f"Downloading {RELEASE_URL} …", file=sys.stderr)
    with urllib.request.urlopen(RELEASE_URL) as resp:
        raw = resp.read()
    return json.loads(gzip.decompress(raw).decode("utf-8"))


def slim(cities: list[dict]) -> list[dict]:
    seen: set[tuple[str, str]] = set()
    out: list[dict] = []
    for c in cities:
        name = (c.get("name") or "").strip()
        country = (c.get("country_name") or "").strip()
        iso2 = (c.get("country_code") or "").strip().upper()
        if not name or not country or not iso2:
            continue
        key = (name.lower(), iso2)
        if key in seen:
            continue
        seen.add(key)
        out.append({"n": name, "c": country, "i": iso2})
    out.sort(key=lambda x: (x["n"].lower(), x["c"].lower()))
    return out


def main() -> None:
    src = sys.argv[1] if len(sys.argv) > 1 else None
    cities = load_cities(src)
    index = slim(cities)
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with gzip.open(OUT, "wt", encoding="utf-8", compresslevel=9) as f:
        json.dump(index, f, ensure_ascii=False, separators=(",", ":"))
    print(f"Wrote {len(index)} cities → {OUT} ({os.path.getsize(OUT):,} bytes)")


if __name__ == "__main__":
    main()
