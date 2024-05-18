package driver_test

import (
	"context"
	"testing"

	// Assuming your package is named driver and resides in the same module
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/joejulian/csi-justmount/pkg/driver"
)

// Example function to test initialization of a driver
func TestNewDriver(t *testing.T) {
	// Arrange
	endpoint := "/tmp/testendpoint"

	// Act
	drv := driver.NewFakeJustMountDriver(endpoint)

	// Assert
	if drv.Endpoint != endpoint {
		t.Errorf("Expected Name to be %s, but got %s", endpoint, drv.Endpoint)
	}

	ctx := context.TODO()
	_, err := drv.NodePublishVolume(
		ctx,
		&csi.NodePublishVolumeRequest{
			VolumeId:          "foo",
			StagingTargetPath: "/staging/foo",
			TargetPath:        "/pod/foo",
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
			},
			Readonly: false,
		},
	)
	if err != nil {
		t.Errorf("Expected success in NodePublishVolume: %v", err)
	}

	_, err = drv.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "foo",
		TargetPath: "/pod/foo",
	})
	if err != nil {
		t.Errorf("Expected success in NodeUnpublishVolume: %v", err)
	}

}
