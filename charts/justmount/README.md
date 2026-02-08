# justmount Helm Chart

Node-only CSI driver for mounting existing filesystems.

## Install

```bash
helm install justmount ./charts/justmount --namespace kube-system
```

## OCI Registry

On releases, the chart is published to GHCR:

```
oci://ghcr.io/<owner>/charts
```

Example install:

```bash
helm install justmount oci://ghcr.io/<owner>/charts/justmount --namespace kube-system
```

## Values

Key values (see `values.yaml` for the full list):

- `image.repository`
- `image.tag` (defaults to `appVersion` when empty)
- `node.endpoint`
- `node.kubeletDir`
- `csidriver.name`
