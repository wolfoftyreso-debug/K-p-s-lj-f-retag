"""Check repository-local Markdown links without reading ignored tool/data files."""

from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlparse

root = Path(__file__).resolve().parent.parent
result = subprocess.run(
    ["git", "ls-files", "--cached", "--others", "--exclude-standard", "--", "*.md"],
    cwd=root,
    text=True,
    capture_output=True,
    check=True,
)
checked = 0
errors = []
for name in sorted(set(result.stdout.splitlines())):
    source = root / name
    for match in re.finditer(r"\]\(([^)]+)\)", source.read_text(encoding="utf-8")):
        target = match.group(1).strip().strip("<>")
        if urlparse(target).scheme or target.startswith("#"):
            continue
        target = unquote(target.split("#", 1)[0])
        if not target:
            continue
        checked += 1
        if not (source.parent / target).resolve().exists():
            errors.append(f"{name}: missing local link {target}")
if errors:
    print("\n".join(errors), file=sys.stderr)
    sys.exit(1)
print(f"PASS {checked} repository-local Markdown links")
