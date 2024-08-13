package control

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) LimitRanges() limitranges {
	return limitranges{
		ck.clientset,
		context.TODO(),
	}
}

type limitranges struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t limitranges) Get(namespace, name string) (*v1.LimitRange, error) {
	return t.clientset.CoreV1().LimitRanges(namespace).Get(t.ctx, name, metav1.GetOptions{})
}

func (t limitranges) Delete(namespace, name string) error {
	return t.clientset.CoreV1().LimitRanges(namespace).Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (t limitranges) Apply(namespace, name string, spec *corev1.LimitRangeSpecApplyConfiguration) (result *v1.LimitRange, err error) {
	return t.clientset.CoreV1().LimitRanges(namespace).Apply(t.ctx, &corev1.LimitRangeApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				v := "LimitRange"
				return &v
			}(),
			APIVersion: func() *string {
				v := "v1"
				return &v
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name:      &name,
			Namespace: &namespace,
		},
		Spec: spec,
	}, metav1.ApplyOptions{FieldManager: "kok"})
}
