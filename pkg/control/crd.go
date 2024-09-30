package control

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type crd struct {
	namesapce     string
	dynamicClient *dynamic.DynamicClient
	clientset     kubernetes.Interface
	ctx           context.Context
}

func (ck Kc) Crd(namesapce string) crd {
	return crd{
		namesapce,
		ck.dynamicClient,
		ck.clientset,
		context.TODO(),
	}
}

func (t crd) Apply(name string, object map[string]interface{}, gvr schema.GroupVersionResource) error {
	obj := &unstructured.Unstructured{
		Object: object,
	}
	_, err := t.dynamicClient.Resource(gvr).Namespace(t.namesapce).Apply(t.ctx, name, obj, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	return err
}

func (t crd) Delete(name string, grv schema.GroupVersionResource) error {
	return t.dynamicClient.Resource(grv).Namespace(t.namesapce).Delete(t.ctx, name, metav1.DeleteOptions{})
}
