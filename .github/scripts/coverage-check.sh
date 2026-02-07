#!/usr/bin/env bash
set -euo pipefail

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

coverage_total() {
  local profile="$1"
  go tool cover -func="${profile}" | awk '/^total:/ {print substr($3, 1, length($3)-1)}'
}

current_cov_profile="${TMP_DIR}/coverage.current.out"
go test ./... -coverprofile="${current_cov_profile}" >/dev/null
current_cov="$(coverage_total "${current_cov_profile}")"

base_ref="${GITHUB_BASE_REF:-}"
if [[ -z "${base_ref}" ]]; then
  if git rev-parse HEAD^ >/dev/null 2>&1; then
    base_ref="HEAD^"
  else
    echo "No base ref available; skipping coverage comparison."
    echo "Current coverage: ${current_cov}%"
    exit 0
  fi
else
  base_ref="refs/remotes/origin/${base_ref}"
  git fetch origin "refs/heads/${GITHUB_BASE_REF}:${base_ref}" >/dev/null 2>&1 || true
fi

git worktree add -q "${TMP_DIR}/base" "${base_ref}"
pushd "${TMP_DIR}/base" >/dev/null
base_cov_profile="${TMP_DIR}/coverage.base.out"
if ! go test ./... -coverprofile="${base_cov_profile}" >/dev/null; then
  echo "Base tests failed; skipping coverage comparison."
  echo "Current coverage: ${current_cov}%"
  popd >/dev/null
  exit 0
fi
base_cov="$(coverage_total "${base_cov_profile}")"
popd >/dev/null

echo "Base coverage: ${base_cov}%"
echo "Current coverage: ${current_cov}%"

if awk -v base="${base_cov}" -v cur="${current_cov}" 'BEGIN {exit (cur+0 < base+0) ? 0 : 1}'; then
  echo "Coverage dropped: ${base_cov}% -> ${current_cov}%"
  exit 1
fi
