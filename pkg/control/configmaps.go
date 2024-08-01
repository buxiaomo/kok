package control

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) ConfigMaps() configmap {
	return configmap{
		ck.clientset,
		context.TODO(),
	}
}

type configmap struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t configmap) Get(namespace, name string) (*v1.ConfigMap, error) {
	return t.clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t configmap) Delete(namespace, name string) error {
	return t.clientset.CoreV1().ConfigMaps(namespace).Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (t configmap) Apply(namespace, name string, data map[string]string) (result *v1.ConfigMap, err error) {
	return t.clientset.CoreV1().ConfigMaps(namespace).Apply(t.ctx, &applycorev1.ConfigMapApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				kind := "ConfigMap"
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
		Data: data,
	}, metav1.ApplyOptions{FieldManager: "kok"})

}
