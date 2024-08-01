package control

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type namespace struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (ck Kc) Namespace() namespace {
	return namespace{
		ck.clientset,
		context.TODO(),
	}
}

func (t namespace) List(listOptions metav1.ListOptions) (*v1.NamespaceList, error) {
	return t.clientset.CoreV1().Namespaces().List(context.TODO(), listOptions)
}

func (t namespace) Get(name string) (*v1.Namespace, error) {
	return t.clientset.CoreV1().Namespaces().Get(t.ctx, name, metav1.GetOptions{})

}
func (ns namespace) Patch(name string, pt types.PatchType, data []byte) (result *v1.Namespace, err error) {
	return ns.clientset.CoreV1().Namespaces().Patch(ns.ctx, name, pt, data, metav1.PatchOptions{})
}

func (ns namespace) Delete(name string) error {
	return ns.clientset.CoreV1().Namespaces().Delete(ns.ctx, name, metav1.DeleteOptions{})
}
func (ns namespace) Apply(ObjectMeta *applymetav1.ObjectMetaApplyConfiguration) (result *v1.Namespace, err error) {
	return ns.clientset.CoreV1().Namespaces().Apply(ns.ctx, &applycorev1.NamespaceApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				a := "Namespace"
				return &a
			}(),
			APIVersion: func() *string {
				a := "v1"
				return &a
			}(),
		},
		ObjectMetaApplyConfiguration: ObjectMeta,
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
}
