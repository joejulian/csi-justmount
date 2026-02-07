# Examples

This directory contains example Kubernetes manifests for using the justmount CSI driver
with static PersistentVolumes.

- `gluster-static.yaml`: Static PV/PVC/Pod example mounting a GlusterFS volume and
  using `subPath` to bind-mount a subdirectory into the pod.
- `kind/justmount-node.yaml`: Node-only DaemonSet and CSIDriver for KIND.
- `kind/tmpfs-static.yaml`: Static PV/PVC/Pod example using tmpfs in KIND.
- `kind/cleanup.sh`: Cleanup script for KIND resources (uses `KUBECONFIG=/tmp/k` by default).
