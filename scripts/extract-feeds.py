#!/usr/bin/env python3
"""Extract monthly primary feeds from the raw traffic-authority archives.

The authority forwards each month as a zip of ~9 Excel/PDF reports (often with
non-UTF8 Arabic filenames that macOS refuses to write). This picks only the one
file we ingest — the "brands × models by vehicle status" primary feed — from
each zip, identifies its period from the title row, and writes it to a clean
name  feeds/<YYYY>-<MM>.xlsx  (deduping repeats).

Usage:  python3 scripts/extract-feeds.py [dump_dir=dump] [out_dir=feeds]

Then load everything:  ./bin/server migrate-facts && ./bin/server load-feeds feeds/
"""
import sys, os, re, glob, zipfile, shutil, tempfile
import openpyxl

DUMP = sys.argv[1] if len(sys.argv) > 1 else "dump"
OUT = sys.argv[2] if len(sys.argv) > 2 else "feeds"
NEED = ["محافظة الإصدار", "المنفذ", "الماركة", "الطراز"]


def primary_period(path):
    """Return (year, month) if the first sheet is the primary status feed, else None."""
    try:
        ws = openpyxl.load_workbook(path, read_only=True, data_only=True).worksheets[0]
    except Exception:
        return None
    it = ws.iter_rows(values_only=True)
    try:
        r0, r1, r2 = next(it), next(it), next(it)
    except StopIteration:
        return None
    header = "|".join(str(c) for c in (r1 or []) if c)
    sub = "|".join(str(c) for c in (r2 or []) if c)
    if not (all(n in header for n in NEED) and "Zero" in sub and "Used" in sub):
        return None
    m = re.search(r"من\s*(\d{4})/(\d{1,2})/(\d{1,2})", str((r0 or [None])[0] or ""))
    return (int(m.group(1)), int(m.group(2))) if m else None


def main():
    os.makedirs(OUT, exist_ok=True)
    tmp = tempfile.mkdtemp()
    written = []
    # zips (monthly bundles) + any loose .xlsx already in the dump
    sources = sorted(glob.glob(os.path.join(DUMP, "*.zip")))
    for z in sources:
        try:
            zf = zipfile.ZipFile(z)
        except Exception as e:
            print(f"  skip {os.path.basename(z)}: {e}"); continue
        for name in zf.namelist():
            b = os.path.basename(name)
            if not name.lower().endswith(".xlsx") or "__MACOSX" in name or b.startswith("._"):
                continue
            probe = os.path.join(tmp, "probe.xlsx")
            with open(probe, "wb") as fh:
                fh.write(zf.read(name))
            per = primary_period(probe)
            if per:
                dest = os.path.join(OUT, f"{per[0]}-{per[1]:02d}.xlsx")
                if os.path.exists(dest):
                    print(f"  {os.path.basename(z):16} {per} -> skip (dup)")
                else:
                    shutil.copy(probe, dest)
                    written.append(dest)
                    print(f"  {os.path.basename(z):16} {per} -> {dest}")
                break  # one primary feed per zip
    print(f"\nextracted {len(written)} feed(s) to {OUT}/")


if __name__ == "__main__":
    main()
