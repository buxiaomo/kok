package control

import (
	"context"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type daemonset struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (ck Kc) DaemonSets() daemonset {
	return daemonset{
		ck.clientset,
		context.TODO(),
	}
}

func (t daemonset) Get(namespace, name string) (result *v1.DaemonSet, err error) {
	return t.clientset.AppsV1().DaemonSets(namespace).Get(t.ctx, name, metav1.GetOptions{})
}
