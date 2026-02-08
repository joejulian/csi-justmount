#!/usr/bin/env bash
set -euo pipefail

version="${1:?version required}"
tag="${2:?tag required}"
dest="/tmp/justmount-helm"

helm package charts/justmount --version "${version}" --app-version "${tag}" --destination "${dest}"

if [[ "${GORELEASER_SKIP_PUBLISH:-}" == "true" ]]; then
  echo "Skipping helm push (GORELEASER_SKIP_PUBLISH=true)."
  exit 0
fi

helm push "${dest}/justmount-${version}.tgz" "oci://ghcr.io/${GITHUB_REPOSITORY_OWNER}/charts"
