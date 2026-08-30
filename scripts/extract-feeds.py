#!/usr/bin/env python3
"""Extract monthly feeds from the raw traffic-authority archives.

The authority forwards each month as a zip of ~14 Excel/PDF reports (often with
non-UTF8 Arabic filenames that macOS refuses to write). This picks only the ones
we ingest, identifies each file's period from its title row, and writes it to a
clean name  <out_dir>/<YYYY>-<MM>.xlsx  (deduping repeats).

Two feed shapes are supported:

  year   (default)  إحصائية الماركات والطرازات للمركبات الزيرو
                    brands × models × year of manufacture, zero registrations.
                    Preferred: it is an exact decomposition of the by-status
                    feed's Zero column and carries سنة الصنع.

  status            إحصائية الماركات والطرازات وفقاً لحالة المركبة
                    the older brands × models × vehicle-status feed. No
                    manufacture year; kept as a fallback.

Each archive also contains a look-alike of the year report scoped to private
plates only (تقرير … المركبات الملاكي …). It has an identical header signature
and roughly a third of the units, so it is rejected explicitly on the title.

Usage:  python3 scripts/extract-feeds.py [dump_dir=dump] [out_dir=feeds-year] [--kind year|status]

Then load everything:  ./bin/server migrate-facts && ./bin/server load-feeds feeds-year/
"""
import sys, os, re, glob, zipfile, shutil, tempfile
import openpyxl

DIMS = ["محافظة الإصدار", "المنفذ", "الماركة"]
PRIVATE_PLATE = "المركبات الملاكي"


def probe(path, kind):
    """Return (year, month) if `path` is the requested feed shape, else None."""
    try:
        ws = openpyxl.load_workbook(path, read_only=True, data_only=True).worksheets[0]
    except Exception:
        return None
    it = ws.iter_rows(values_only=True)
    try:
        r0, r1, r2 = next(it), next(it), next(it)
    except StopIteration:
        return None
    title = str((r0 or [None])[0] or "")
    header = "|".join(str(c) for c in (r1 or []) if c)
    sub = [c for c in (r2 or []) if c is not None]
    if not all(n in header for n in DIMS):
        return None

    if kind == "year":
        if "سنة الصنع" not in header:
            return None
        # the private-plate twin shares this signature exactly — reject by title
        if PRIVATE_PLATE in title:
            return None
        if not any(isinstance(c, (int, float)) and 1980 <= int(c) <= 2100 for c in sub):
            return None
    else:
        subtext = "|".join(str(c) for c in sub)
        if "Zero" not in subtext or "Used" not in subtext:
            return None

    m = re.search(r"من\s*(\d{4})/(\d{1,2})/(\d{1,2})", title)
    return (int(m.group(1)), int(m.group(2))) if m else None


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    kind = "year"
    for a in sys.argv[1:]:
        if a.startswith("--kind"):
            kind = a.split("=", 1)[1] if "=" in a else "year"
    if "--kind" in sys.argv:
        i = sys.argv.index("--kind")
        if i + 1 < len(sys.argv):
            kind = sys.argv[i + 1]
    if kind not in ("year", "status"):
        sys.exit(f"unknown --kind {kind!r} (expected year|status)")

    dump = args[0] if len(args) > 0 else "dump"
    out = args[1] if len(args) > 1 else ("feeds-year" if kind == "year" else "feeds")

    os.makedirs(out, exist_ok=True)
    tmp = tempfile.mkdtemp()
    written = []
    for z in sorted(glob.glob(os.path.join(dump, "*.zip"))):
        try:
            zf = zipfile.ZipFile(z)
        except Exception as e:
            print(f"  skip {os.path.basename(z)}: {e}")
            continue
        # Probe EVERY xlsx: an archive holds several sheets that share a header
        # shape, so stopping at the first match picks up the wrong one.
        for name in zf.namelist():
            b = os.path.basename(name)
            if not name.lower().endswith(".xlsx") or "__MACOSX" in name or b.startswith("._"):
                continue
            probe_path = os.path.join(tmp, "probe.xlsx")
            with open(probe_path, "wb") as fh:
                fh.write(zf.read(name))
            per = probe(probe_path, kind)
            if not per:
                continue
            dest = os.path.join(out, f"{per[0]}-{per[1]:02d}.xlsx")
            if os.path.exists(dest):
                print(f"  {os.path.basename(z):16} {per} -> skip (already have it)")
            else:
                shutil.copy(probe_path, dest)
                written.append(dest)
                print(f"  {os.path.basename(z):16} {per} -> {dest}")
            break  # one feed of this kind per archive

    print(f"\nextracted {len(written)} {kind} feed(s) to {out}/")


if __name__ == "__main__":
    main()
