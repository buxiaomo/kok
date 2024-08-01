package control

import (
	"context"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) StatefulSets() statefulsets {
	return statefulsets{
		ck.clientset,
		context.TODO(),
	}
}

type statefulsets struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t statefulsets) Patch(namespace, name string, data []byte) (result *v1.StatefulSet, err error) {
	return t.clientset.AppsV1().StatefulSets(namespace).Patch(context.TODO(), name, types.MergePatchType, data, metav1.PatchOptions{})
}

func (t statefulsets) Delete(namespace, name string) error {
	return t.clientset.AppsV1().StatefulSets(namespace).Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (t statefulsets) Apply(namespace, name string, spec *applyappsv1.StatefulSetSpecApplyConfiguration) (result *v1.StatefulSet, err error) {
	return t.clientset.AppsV1().StatefulSets(namespace).Apply(t.ctx, &applyappsv1.StatefulSetApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "StatefulSet"
				return &name
			}(),
			APIVersion: func() *string {
				name := "apps/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name:      &name,
			Namespace: &namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: spec,
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
}
