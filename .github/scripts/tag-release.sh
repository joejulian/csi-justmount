#!/usr/bin/env bash
set -euo pipefail

current_version="$(python3 - <<'PY'
import json
from pathlib import Path

path = Path(".release-please-manifest.json")
if not path.exists():
    print("0.0.0")
    raise SystemExit
data = json.loads(path.read_text())
print(data.get(".", "0.0.0"))
PY
)"

latest_tag="$(git tag --list "v*" --sort=-v:refname | head -n1)"

if [[ -n "${latest_tag}" ]]; then
  log_range="${latest_tag}..HEAD"
else
  log_range="HEAD"
fi

next_version="$(python3 - <<PY
import re
import subprocess

current = "${current_version}"
log_range = "${log_range}"

def git(*args: str) -> str:
    return subprocess.check_output(["git", *args], text=True)

raw = git("log", log_range, "--format=%s%n%b%n==END==")
commits = [c.strip() for c in raw.split("==END==") if c.strip()]

def is_release_commit(subject: str) -> bool:
    return bool(re.match(r"^chore\\(release\\):\\s*v?\\d+\\.\\d+\\.\\d+", subject))

def parse_subject(subject: str):
    # Conventional commits: type(scope)!: subject
    m = re.match(r"^(?P<type>\\w+)(?:\\([^)]*\\))?(?P<breaking>!)?:\\s+", subject)
    if not m:
        return None, False
    return m.group("type"), bool(m.group("breaking"))

major = minor = patch = False
for c in commits:
    lines = c.splitlines()
    subject = lines[0].strip() if lines else ""
    body = "\n".join(lines[1:])
    if is_release_commit(subject):
        continue
    ctype, breaking = parse_subject(subject)
    if ctype not in {"feat", "fix", "perf"}:
        continue
    if breaking or "BREAKING CHANGE" in body:
        major = True
        continue
    if ctype == "feat":
        minor = True
        continue
    if ctype in {"fix", "perf"}:
        patch = True

if not (major or minor or patch):
    print("")
    raise SystemExit

parts = current.split(".")
major_v = int(parts[0]) if len(parts) > 0 else 0
minor_v = int(parts[1]) if len(parts) > 1 else 0
patch_v = int(parts[2]) if len(parts) > 2 else 0

if major:
    major_v += 1
    minor_v = 0
    patch_v = 0
elif minor:
    minor_v += 1
    patch_v = 0
else:
    patch_v += 1

print(f"{major_v}.{minor_v}.{patch_v}")
PY
)"

if [[ -z "${next_version}" ]]; then
  echo "No release-worthy commits found."
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "created=false" >> "${GITHUB_OUTPUT}"
  fi
  exit 0
fi

tag="v${next_version}"

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  echo "Tag ${tag} already exists."
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "created=false" >> "${GITHUB_OUTPUT}"
  fi
  exit 0
fi

python3 - <<PY
import json
from pathlib import Path

path = Path(".release-please-manifest.json")
data = json.loads(path.read_text())
data["."] = "${next_version}"
path.write_text(json.dumps(data, indent=2) + "\n")
PY

perl -0pi -e "s/^appVersion: .*$/appVersion: ${next_version}/m" charts/justmount/Chart.yaml

git add .release-please-manifest.json charts/justmount/Chart.yaml
git commit -m "chore(release): ${tag}"
git tag "${tag}"

git push origin HEAD:main
git push origin "${tag}"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "created=true" >> "${GITHUB_OUTPUT}"
  echo "tag=${tag}" >> "${GITHUB_OUTPUT}"
fi
