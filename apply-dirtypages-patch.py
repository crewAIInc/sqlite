#!/usr/bin/env python3
"""Insert the dirty-page capture call site into the transpiled pager.

lib/sqlite_<goos>_<goarch>.go is regenerated wholesale each release, so the
patch is a one-line insertion this script re-applies rather than a diff that
has to be rebased. Run it from a checkout root after taking a new upstream
tag; it is idempotent.

    ./apply-dirtypages-patch.py [path-to-checkout]

The anchor is the handoff to the WAL layer, matched as text: the same
declaration sits at four different line numbers across the targets, and
regeneration moves all of them.

Exit codes: 0 patched or already patched, 1 the anchor moved or stopped being
unique, 2 bad arguments. Targets carrying no anchor are reported and skipped —
freebsd/386, freebsd/arm and netbsd/amd64 predate the generator that produces
_pagerWalFrames and are excluded by the build constraint on
lib/dirtypages_capture.go. A target that gains the anchor while that constraint
still excludes it fails to build, which is the intended outcome.
"""

import glob
import os
import sys

ANCHOR = "\trc = _sqlite3WalFrames(tls, (*TPager)(unsafe.Pointer(pPager)).FpWal"
INSERT = "\tcallDirtyPagesHook(tls, pPager, *(*uintptr)(unsafe.Pointer(bp)), nTruncate, isCommit)\n"


def main(argv):
    if len(argv) > 2:
        print(__doc__.strip(), file=sys.stderr)
        return 2

    root = os.path.abspath(argv[1] if len(argv) == 2 else ".")
    lib = os.path.join(root, "lib")
    if not os.path.isdir(lib):
        print(f"not a modernc.org/sqlite checkout (no lib/): {root}", file=sys.stderr)
        return 2
    for name in ("dirtypages.go", "dirtypages_capture.go", "dirtypages_nocapture.go"):
        if not os.path.isfile(os.path.join(lib, name)):
            print(f"missing lib/{name}; the hook body is not in this checkout", file=sys.stderr)
            return 2

    patched, already, no_anchor, ambiguous = [], [], [], []

    for path in sorted(glob.glob(os.path.join(lib, "sqlite_*.go"))):
        name = os.path.basename(path)
        with open(path) as f:
            text = f.read()
        if INSERT.strip() in text:
            already.append(name)
            continue
        n = text.count(ANCHOR)
        if n == 0:
            no_anchor.append(name)
            continue
        if n != 1:
            ambiguous.append((name, n))
            continue
        os.chmod(path, 0o644)
        with open(path, "w") as f:
            f.write(text.replace(ANCHOR, INSERT + ANCHOR, 1))
        patched.append(name)

    print(f"patched {len(patched)}, already patched {len(already)}, no anchor {len(no_anchor)}")
    for name in no_anchor:
        print(f"  skipped (no anchor): {name}")
    for name, n in ambiguous:
        print(f"  ANCHOR NOT UNIQUE in {name}: {n} occurrences", file=sys.stderr)

    if ambiguous:
        return 1
    if not patched and not already:
        print("no target carried the anchor — the call site moved", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
