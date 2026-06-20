package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

const (
	podNameContextKey      = "csi.storage.k8s.io/pod.name"
	podNamespaceContextKey = "csi.storage.k8s.io/pod.namespace"
	pvcRepairConditionType = corev1.PersistentVolumeClaimConditionType("JustmountVolumeRepairing")
)

// PVCReporter records justmount repair state on the affected PersistentVolumeClaim.
type PVCReporter interface {
	RepairStarted(ctx context.Context, req *csi.NodePublishVolumeRequest, reason, message string) error
	RepairCompleted(ctx context.Context, req *csi.NodePublishVolumeRequest, reason, message string) error
}

type KubernetesPVCReporter struct {
	client     kubernetes.Interface
	nodeID     string
	driverName string
}

type pvcRef struct {
	namespace string
	name      string
	uid       types.UID
}

// NewKubernetesPVCReporter creates a reporter backed by the in-cluster Kubernetes API.
func NewKubernetesPVCReporter(nodeID, driverName string) (*KubernetesPVCReporter, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return &KubernetesPVCReporter{
		client:     client,
		nodeID:     nodeID,
		driverName: driverName,
	}, nil
}

func (r *KubernetesPVCReporter) RepairStarted(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
	reason string,
	message string,
) error {
	ref, err := r.resolvePVC(ctx, req)
	if err != nil {
		return err
	}
	if ref == nil {
		return nil
	}
	if err := r.setCondition(ctx, *ref, corev1.ConditionTrue, reason, message); err != nil {
		return err
	}
	return r.createEvent(ctx, *ref, corev1.EventTypeWarning, reason, message)
}

func (r *KubernetesPVCReporter) RepairCompleted(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
	reason string,
	message string,
) error {
	ref, err := r.resolvePVC(ctx, req)
	if err != nil {
		return err
	}
	if ref == nil {
		return nil
	}
	if err := r.setCondition(ctx, *ref, corev1.ConditionFalse, reason, message); err != nil {
		return err
	}
	return r.createEvent(ctx, *ref, corev1.EventTypeNormal, reason, message)
}

func (r *KubernetesPVCReporter) resolvePVC(ctx context.Context, req *csi.NodePublishVolumeRequest) (*pvcRef, error) {
	volumeID := req.GetVolumeId()
	podName := strings.TrimSpace(req.GetVolumeContext()[podNameContextKey])
	podNamespace := strings.TrimSpace(req.GetVolumeContext()[podNamespaceContextKey])
	if podName == "" || podNamespace == "" {
		return nil, nil
	}

	pod, err := r.client.CoreV1().Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get pod %s/%s: %w", podNamespace, podName, err)
	}

	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		pvc, err := r.client.CoreV1().PersistentVolumeClaims(podNamespace).Get(
			ctx,
			volume.PersistentVolumeClaim.ClaimName,
			metav1.GetOptions{},
		)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("get pvc %s/%s: %w", podNamespace, volume.PersistentVolumeClaim.ClaimName, err)
		}
		if pvc.Spec.VolumeName == "" {
			continue
		}
		pv, err := r.client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("get pv %s: %w", pvc.Spec.VolumeName, err)
		}
		if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != r.driverName {
			continue
		}
		if pv.Spec.CSI.VolumeHandle != volumeID && pv.Name != volumeID {
			continue
		}
		return &pvcRef{
			namespace: pvc.Namespace,
			name:      pvc.Name,
			uid:       pvc.UID,
		}, nil
	}

	return nil, nil
}

func (r *KubernetesPVCReporter) setCondition(
	ctx context.Context,
	ref pvcRef,
	status corev1.ConditionStatus,
	reason string,
	message string,
) error {
	now := metav1.Now()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pvc, err := r.client.CoreV1().PersistentVolumeClaims(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		conditions := append([]corev1.PersistentVolumeClaimCondition(nil), pvc.Status.Conditions...)
		conditions = setPVCCondition(conditions, corev1.PersistentVolumeClaimCondition{
			Type:               pvcRepairConditionType,
			Status:             status,
			LastProbeTime:      now,
			LastTransitionTime: now,
			Reason:             reason,
			Message:            message,
		})
		pvc.Status.Conditions = conditions
		_, err = r.client.CoreV1().PersistentVolumeClaims(ref.namespace).UpdateStatus(ctx, pvc, metav1.UpdateOptions{})
		return err
	})
}

func setPVCCondition(
	conditions []corev1.PersistentVolumeClaimCondition,
	next corev1.PersistentVolumeClaimCondition,
) []corev1.PersistentVolumeClaimCondition {
	for i := range conditions {
		if conditions[i].Type != next.Type {
			continue
		}
		if conditions[i].Status == next.Status {
			next.LastTransitionTime = conditions[i].LastTransitionTime
		}
		conditions[i] = next
		return conditions
	}
	return append(conditions, next)
}

func (r *KubernetesPVCReporter) createEvent(
	ctx context.Context,
	ref pvcRef,
	eventType string,
	reason string,
	message string,
) error {
	now := metav1.NewTime(time.Now())
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "justmount-" + uuid.NewString(),
			Namespace: ref.namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
			Namespace:  ref.namespace,
			Name:       ref.name,
			UID:        ref.uid,
		},
		Reason:              reason,
		Message:             message,
		Type:                eventType,
		Source:              corev1.EventSource{Component: "justmount", Host: r.nodeID},
		ReportingController: r.driverName,
		ReportingInstance:   r.nodeID,
		FirstTimestamp:      now,
		LastTimestamp:       now,
		EventTime:           metav1.MicroTime{Time: now.Time},
		Count:               1,
	}
	_, err := r.client.CoreV1().Events(ref.namespace).Create(ctx, event, metav1.CreateOptions{})
	return err
}
