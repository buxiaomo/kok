package control

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) Pvc() pvc {
	return pvc{
		ck.clientset,
		context.TODO(),
	}
}

type pvc struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t pvc) Delete(namespace, name string) error {
	return t.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (t pvc) Apply(namespace, name string, persistentVolumeClaim *applycorev1.PersistentVolumeClaimSpecApplyConfiguration) (result *v1.PersistentVolumeClaim, err error) {
	return t.clientset.CoreV1().PersistentVolumeClaims(namespace).Apply(t.ctx, &applycorev1.PersistentVolumeClaimApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				kind := "PersistentVolumeClaim"
				return &kind
			}(),
			APIVersion: func() *string {
				kind := "v1"
				return &kind
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name:      &name,
			Namespace: &namespace,
		},
		Spec: persistentVolumeClaim,
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
}
