# justmount Helm Chart

Node-only CSI driver for mounting existing filesystems.

## Install

```bash
helm install justmount ./charts/justmount --namespace kube-system
```

## Values

Key values (see `values.yaml` for the full list):

- `image.repository`
- `image.tag`
- `node.endpoint`
- `node.kubeletDir`
- `csidriver.name`
