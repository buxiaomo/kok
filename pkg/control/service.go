package control

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type service struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t service) Apply(namespace, name string, ServiceSpec *applycorev1.ServiceSpecApplyConfiguration) (result *v1.Service, err error) {
	return t.clientset.CoreV1().Services(namespace).Apply(t.ctx, &applycorev1.ServiceApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				kind := "Service"
				return &kind
			}(),
			APIVersion: func() *string {
				APIVersion := "v1"
				return &APIVersion
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name:      &name,
			Namespace: &namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: ServiceSpec,
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
}

func (t service) Get(namespace, name string) (result *v1.Service, err error) {
	return t.clientset.CoreV1().Services(namespace).Get(t.ctx, name, metav1.GetOptions{})
}

func (t service) Delete(namespace, name string) error {
	return t.clientset.CoreV1().Services(namespace).Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (ck Kc) Service() service {
	return service{
		ck.clientset,
		context.TODO(),
	}
}
