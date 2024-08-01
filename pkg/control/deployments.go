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

func (ck Kc) Deployment() deployment {
	return deployment{
		ck.clientset,
		context.TODO(),
	}
}

type deployment struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t deployment) Get(namespace, name string) (result *v1.Deployment, err error) {
	return t.clientset.AppsV1().Deployments(namespace).Get(t.ctx, name, metav1.GetOptions{})
}

func (t deployment) Patch(namespace, name string, data []byte) (result *v1.Deployment, err error) {

	return t.clientset.AppsV1().Deployments(namespace).Patch(t.ctx, name, types.MergePatchType, data, metav1.PatchOptions{})
}

func (t deployment) Delete(namespace, name string) error {
	return t.clientset.AppsV1().Deployments(namespace).Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (t deployment) Apply(namespace, name string, spec *applyappsv1.DeploymentSpecApplyConfiguration) (result *v1.Deployment, err error) {
	return t.clientset.AppsV1().Deployments(namespace).Apply(t.ctx, &applyappsv1.DeploymentApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				a := "Deployment"
				return &a
			}(),
			APIVersion: func() *string {
				a := "apps/v1"
				return &a
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
