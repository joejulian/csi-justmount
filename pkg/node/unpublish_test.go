package node_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/joejulian/csi-justmount/pkg/node"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNodeUnpublishVolumeRejectsRootTargetPath(t *testing.T) {
	n := node.NewNodeWithMounter("node-id", "/tmp/test-csi.sock", newFakeMounter())

	req := &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "test-volume",
		TargetPath: "/",
	}

	_, err := n.NodeUnpublishVolume(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error for unsafe target path")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %s", st.Code())
	}
}

func TestNodeUnpublishVolumeDoesNotRecursivelyDeleteTarget(t *testing.T) {
	fake := newFakeMounter()
	n := node.NewNodeWithMounter("node-id", "/tmp/test-csi.sock", fake)

	targetPath := t.TempDir()
	child := filepath.Join(targetPath, "child")
	if err := os.WriteFile(child, []byte("data"), 0644); err != nil {
		t.Fatalf("create child: %v", err)
	}

	req := &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "test-volume",
		TargetPath: targetPath,
	}

	_, err := n.NodeUnpublishVolume(context.Background(), req)
	if err == nil {
		t.Fatalf("expected failure removing non-empty directory")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Fatalf("expected Internal, got %s", st.Code())
	}

	if _, statErr := os.Stat(child); statErr != nil {
		t.Fatalf("child entry should still exist; got: %v", statErr)
	}
}
