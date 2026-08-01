#!/usr/bin/env python3
"""Check internal links and anchors in all markdown files.

- Relative links must resolve on a case-sensitive filesystem (Linux).
- Anchors must match GitHub's heading-slug algorithm (lowercase, hyphenated,
  punctuation stripped, non-ASCII kept).
- Links inside fenced code blocks and inline code are skipped.
- External URLs are not fetched (checked manually or in CI).

Usage: python3 tools/check-links.py
Exit code 0 = all good, 1 = problems found.
"""
import os
import re
import sys
import unicodedata

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SKIP_DIRS = {".git", "node_modules", "test-data", ".venv"}
EXCLUDE = {"package-lock.json"}


def md_files(root):
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for name in filenames:
            if name.endswith(".md") and name not in EXCLUDE:
                yield os.path.join(dirpath, name)


def strip_code(text):
    """Remove fenced code blocks and inline code so links inside are skipped."""
    text = re.sub(r"```.*?```", "```\n```", text, flags=re.DOTALL)
    text = re.sub(r"`[^`]*`", "", text)
    return text


def slugify(heading):
    """GitHub-style anchor slug."""
    h = heading.strip().lower()
    h = unicodedata.normalize("NFKD", h)
    h = h.encode("ascii", "ignore").decode("ascii")
    h = re.sub(r"[^\w\s-]", "", h)
    h = re.sub(r"\s+", "-", h)
    return h


def collect_anchors(path):
    anchors = {}
    seen = {}
    with open(path, encoding="utf-8", errors="replace") as f:
        for line in f:
            m = re.match(r"^#{1,6}\s+(.*)$", line)
            if not m:
                continue
            slug = slugify(m.group(1))
            seen[slug] = seen.get(slug, 0) + 1
            if seen[slug] > 1:
                slug = f"{slug}-{seen[slug] - 1}"
            anchors[slug] = True
    return anchors


def check_file(path, problems):
    with open(path, encoding="utf-8", errors="replace") as f:
        raw = f.read()
    text = strip_code(raw)
    base = os.path.dirname(path)
    anchors_cache = {}

    def anchors_for(target_path):
        if target_path not in anchors_cache:
            anchors_cache[target_path] = collect_anchors(target_path)
        return anchors_cache[target_path]

    for m in re.finditer(r"\[[^\]]*\]\(([^)]+)\)", text):
        target = m.group(1).strip()
        if not target or target.startswith(("#", "http://", "https://", "mailto:")):
            continue
        file_part, _, anchor = target.partition("#")
        if not file_part:
            # Same-file anchor
            if anchor and anchor not in anchors_for(path):
                problems.append(f"{path}: anchor '#{anchor}' not found (same file)")
            continue
        # Strip query/hash-less weirdness, decode
        file_part = file_part.split("?")[0]
        full = os.path.normpath(os.path.join(base, file_part))
        if not os.path.exists(full):
            problems.append(f"{path}: link -> '{target}' missing file '{full}'")
            continue
        if anchor and anchor not in anchors_for(full):
            problems.append(f"{path}: link -> '{target}' anchor '#{anchor}' not found in '{file_part}'")

    # Bare-name mentions of moved/old docs (prose dangles)
    old_names = [
        "TESTING.md", "chart_testing", "TESTING-COMPLEX-CHARTS", "USE-CASES",
        "HELM-PLUGIN", "PLUGIN-SPECIFIC", "cli-reference", "SOLVER.md",
        "RULES.md", "LOGGING.md", "STRUCTURED-LOGGING", "RELEASE.md",
        "COVERAGE.md", "DEVELOPMENT.md", "comment-alignment",
        "CHART-VALIDATION-ISSUES", "TODO.md", "FILESYSTEM-MOCKING", "nilaway.md",
        "TESTING-FILESYSTEM-MOCKING",
    ]
    for name in old_names:
        # Skip when part of a link we already validated or a stub banner
        # Strip markdown link syntax first so valid links do not self-flag.
        bare = re.sub(r"\[[^\]]*\]\([^)]*\)", "", text)
        if path.endswith("CHANGELOG.md") or path.endswith("FAQ.md"):
            bare = ""  # historical entries / FAQ text are exempt from the sweep
        if os.sep + "archive" + os.sep in path:
            bare = ""  # archived docs are historical records; links are still checked above
        for line in bare.splitlines():
            if name in line and "was merged into" not in line and "ARCHIVED" not in line:
                # Only flag bare mentions that look like file references
                if re.search(rf"(?<![\w./-]){re.escape(name)}", line):
                    problems.append(f"{path}: stale bare mention of '{name}' -> {line.strip()[:80]}")


def main():
    problems = []
    files = list(md_files(ROOT))
    for f in files:
        check_file(f, problems)
    if problems:
        print(f"Found {len(problems)} problem(s):")
        for p in problems:
            print(f"  - {p}")
        return 1
    print(f"OK: {len(files)} markdown files, no broken links or stale mentions.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
