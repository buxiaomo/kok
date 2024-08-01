package control

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type pod struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t pod) List(namespace string, listOptions metav1.ListOptions) (*v1.PodList, error) {
	return t.clientset.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
}

func (t pod) Delete(namespace, name string) error {
	return t.clientset.CoreV1().Pods(namespace).Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (ck Kc) Pods() pod {
	return pod{
		ck.clientset,
		context.TODO(),
	}
}
