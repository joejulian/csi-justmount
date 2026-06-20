package node

import (
	"context"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesPVCReporterSetsPVCConditionAndEvents(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "app-data",
							},
						},
					},
				},
			},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-data",
				Namespace: "default",
				UID:       types.UID("pvc-uid"),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "app-pv",
			},
		},
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pv"},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						Driver:       driverName,
						VolumeHandle: "volume-handle",
					},
				},
			},
		},
	)
	reporter := &KubernetesPVCReporter{
		client:     client,
		nodeID:     "node-a",
		driverName: driverName,
	}
	req := &csi.NodePublishVolumeRequest{
		VolumeId: "volume-handle",
		VolumeContext: map[string]string{
			podNameContextKey:      "app-pod",
			podNamespaceContextKey: "default",
		},
	}

	if err := reporter.RepairStarted(ctx, req, "JustmountBindMountDisconnected", "repair started"); err != nil {
		t.Fatalf("RepairStarted() error = %v, want nil", err)
	}

	pvc, err := client.CoreV1().PersistentVolumeClaims("default").Get(ctx, "app-data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pvc after RepairStarted(): %v", err)
	}
	condition := findPVCCondition(pvc.Status.Conditions, pvcRepairConditionType)
	if condition == nil {
		t.Fatalf("pvc condition %q missing after RepairStarted()", pvcRepairConditionType)
	}
	if condition.Status != corev1.ConditionTrue {
		t.Fatalf("pvc condition status = %q, want %q", condition.Status, corev1.ConditionTrue)
	}
	if condition.Reason != "JustmountBindMountDisconnected" {
		t.Fatalf("pvc condition reason = %q, want JustmountBindMountDisconnected", condition.Reason)
	}

	if err := reporter.RepairCompleted(ctx, req, "JustmountBindMountReplaced", "repair completed"); err != nil {
		t.Fatalf("RepairCompleted() error = %v, want nil", err)
	}

	pvc, err = client.CoreV1().PersistentVolumeClaims("default").Get(ctx, "app-data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pvc after RepairCompleted(): %v", err)
	}
	condition = findPVCCondition(pvc.Status.Conditions, pvcRepairConditionType)
	if condition == nil {
		t.Fatalf("pvc condition %q missing after RepairCompleted()", pvcRepairConditionType)
	}
	if condition.Status != corev1.ConditionFalse {
		t.Fatalf("pvc condition status = %q, want %q", condition.Status, corev1.ConditionFalse)
	}
	if condition.Reason != "JustmountBindMountReplaced" {
		t.Fatalf("pvc condition reason = %q, want JustmountBindMountReplaced", condition.Reason)
	}

	events, err := client.CoreV1().Events("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 2 {
		t.Fatalf("event count = %d, want 2", len(events.Items))
	}
	eventTypes := make(map[string]string, len(events.Items))
	for _, event := range events.Items {
		eventTypes[event.Reason] = event.Type
	}
	if eventTypes["JustmountBindMountDisconnected"] != corev1.EventTypeWarning {
		t.Fatalf("repair start event type = %q, want %q", eventTypes["JustmountBindMountDisconnected"], corev1.EventTypeWarning)
	}
	if eventTypes["JustmountBindMountReplaced"] != corev1.EventTypeNormal {
		t.Fatalf("repair completed event type = %q, want %q", eventTypes["JustmountBindMountReplaced"], corev1.EventTypeNormal)
	}
}

func TestKubernetesPVCReporterNoPodInfoIsNoop(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	reporter := &KubernetesPVCReporter{
		client:     client,
		nodeID:     "node-a",
		driverName: driverName,
	}

	if err := reporter.RepairStarted(ctx, &csi.NodePublishVolumeRequest{VolumeId: "volume-handle"}, "Reason", "message"); err != nil {
		t.Fatalf("RepairStarted() error = %v, want nil", err)
	}
	events, err := client.CoreV1().Events("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 0 {
		t.Fatalf("event count = %d, want 0", len(events.Items))
	}
}

func TestKubernetesPVCReporterSkipsNonJustmountVolume(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "app-data",
							},
						},
					},
				},
			},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-data",
				Namespace: "default",
				UID:       types.UID("pvc-uid"),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "app-pv",
			},
		},
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pv"},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						Driver:       "other.csi.driver",
						VolumeHandle: "volume-handle",
					},
				},
			},
		},
	)
	reporter := &KubernetesPVCReporter{
		client:     client,
		nodeID:     "node-a",
		driverName: driverName,
	}
	req := &csi.NodePublishVolumeRequest{
		VolumeId: "volume-handle",
		VolumeContext: map[string]string{
			podNameContextKey:      "app-pod",
			podNamespaceContextKey: "default",
		},
	}

	if err := reporter.RepairStarted(ctx, req, "JustmountBindMountDisconnected", "repair started"); err != nil {
		t.Fatalf("RepairStarted() error = %v, want nil", err)
	}
	pvc, err := client.CoreV1().PersistentVolumeClaims("default").Get(ctx, "app-data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pvc: %v", err)
	}
	if condition := findPVCCondition(pvc.Status.Conditions, pvcRepairConditionType); condition != nil {
		t.Fatalf("pvc condition = %#v, want nil", condition)
	}
}

func TestSetPVCConditionPreservesTransitionTimeForUnchangedStatus(t *testing.T) {
	oldTransition := metav1.Now()
	newProbe := metav1.NewTime(oldTransition.Add(1))
	conditions := []corev1.PersistentVolumeClaimCondition{
		{
			Type:               pvcRepairConditionType,
			Status:             corev1.ConditionTrue,
			LastProbeTime:      oldTransition,
			LastTransitionTime: oldTransition,
			Reason:             "OldReason",
			Message:            "old message",
		},
	}

	got := setPVCCondition(conditions, corev1.PersistentVolumeClaimCondition{
		Type:               pvcRepairConditionType,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      newProbe,
		LastTransitionTime: newProbe,
		Reason:             "NewReason",
		Message:            "new message",
	})

	if len(got) != 1 {
		t.Fatalf("condition count = %d, want 1", len(got))
	}
	if !got[0].LastTransitionTime.Equal(&oldTransition) {
		t.Fatalf("LastTransitionTime = %v, want %v", got[0].LastTransitionTime, oldTransition)
	}
	if !got[0].LastProbeTime.Equal(&newProbe) {
		t.Fatalf("LastProbeTime = %v, want %v", got[0].LastProbeTime, newProbe)
	}
	if got[0].Reason != "NewReason" {
		t.Fatalf("Reason = %q, want NewReason", got[0].Reason)
	}
}

func TestNewKubernetesPVCReporterReturnsErrorOutsideCluster(t *testing.T) {
	if reporter, err := NewKubernetesPVCReporter("node-a", driverName); err == nil {
		t.Fatalf("NewKubernetesPVCReporter() reporter = %#v, error = nil; want error outside cluster", reporter)
	}
}

func findPVCCondition(
	conditions []corev1.PersistentVolumeClaimCondition,
	conditionType corev1.PersistentVolumeClaimConditionType,
) *corev1.PersistentVolumeClaimCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
