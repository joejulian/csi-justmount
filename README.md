
# Justmount CSI Driver

The **Justmount CSI Driver** is a Container Storage Interface (CSI) driver designed to provide basic storage management functionality to Kubernetes. This driver provides a Node service only and supports essential CSI node operations such as staging, publishing, unmounting, and unpublishing volumes.

## Features

- **Node Service**: Manages node-local operations, including mounting and unmounting volumes.
- **CSI Compliance**: Compatible with Kubernetes and other orchestrators that support CSI.
- **Sanity Testing**: Includes `csi-sanity` tests to validate CSI functionality and compliance.

## Prerequisites

- **Kubernetes** (>= 1.13) with CSI enabled
- **Go** (>= 1.16) for building and testing the driver
- **csi-sanity** for validating CSI compliance

## Installation

### Building the Driver

To build the Justmount CSI Driver, use the following commands:

```bash
git clone https://github.com/yourusername/justmount.git
cd justmount
go build -o bin/justmount main.go
```

This command will compile the driver binary as `bin/justmount`.

### Configuration

Justmount supports configuration through command-line flags:

- `--node-endpoint`: Path to the Node service socket (default: `/tmp/csi-node.sock`)
- `--node-id`: Unique identifier for each node (required for the Node service)

### Volume Attributes

When using static PVs, the driver expects the following `volumeAttributes` (from the PV `spec.csi.volumeAttributes`) for staging:

- `source` (required): Source passed to the mount call (example: `gluster:media`)
- `fsType` (optional if set in VolumeCapability): Filesystem type (example: `glusterfs`)
- `mountOptions` (optional): Comma-separated mount options (example: `rw,nosuid,nodev`)
- `fileMode` (required): Octal permissions to apply after staging (example: `0755`)

### Deploying on Kubernetes

1. **Install the CSI Driver**:
   Deploy the driver using a DaemonSet for the Node service. 

2. **Configure Storage Classes**:
   Set up a `StorageClass` that references the Justmount CSI driver:

   ```yaml
   apiVersion: storage.k8s.io/v1
   kind: StorageClass
   metadata:
     name: justmount-sc
   provisioner: justmount.csi.driver
   ```

## Usage

After deploying, you can create PersistentVolumeClaims (PVCs) that use the configured StorageClass. Justmount will automatically handle volume attachment, mounting, and unmounting for existing volumes.

### Local vs Network Filesystems

For local filesystems, ensure pods are scheduled on the owning node by setting PV `nodeAffinity`.
For network filesystems, omit `nodeAffinity` so pods can be scheduled anywhere.

Example local PV node affinity:

```yaml
spec:
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: kubernetes.io/hostname
          operator: In
          values:
          - <node-name>
```

### Example PVC

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: justmount-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: justmount-sc
```

## Testing

To validate the driver’s functionality and compliance, run the following command:

```bash
ginkgo ./...
```

This command will execute the `csi-sanity` tests configured in `sanity_test.go` as well as any other unit tests.

## Development

### Running Locally

1. **Build the driver**:

   ```bash
   go build -o bin/justmount main.go
   ```

2. **Run the driver locally**:
   Start the driver with the Node service:

   ```bash
   ./bin/justmount --node-endpoint /tmp/csi-node.sock --node-id <node-id>
   ```

### Running Unit Tests

Run unit tests using:

```bash
go test ./pkg/...
```

### Directory Structure

- `main.go`: Main entry point for the driver.
- `pkg/controller`: Contains Controller service code.
- `pkg/node`: Contains Node service code.
- `sanity_test.go`: Test configuration for `csi-sanity`.

## License

This project is licensed under the GNU General Public License, Version 3 (GPL-3.0). See the `LICENSE` file for details.
