#!/usr/bin/env bash
set -euo pipefail

: "${KUBECONFIG:=/tmp/k}"

kubectl delete -f examples/kind/tmpfs-static.yaml --ignore-not-found
kubectl delete -f examples/kind/justmount-node.yaml --ignore-not-found
