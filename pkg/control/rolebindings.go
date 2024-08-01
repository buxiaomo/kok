package control

import (
	"context"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	applyrbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) RoleBindings() rolebindings {
	return rolebindings{
		ck.clientset,
		context.TODO(),
	}
}

type rolebindings struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t rolebindings) Apply(namespace, name string, Subjects []applyrbacv1.SubjectApplyConfiguration, RoleRef *applyrbacv1.RoleRefApplyConfiguration) (result *v1.RoleBinding, err error) {
	return t.clientset.RbacV1().RoleBindings(namespace).Apply(t.ctx, &applyrbacv1.RoleBindingApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "RoleBinding"
				return &name
			}(),
			APIVersion: func() *string {
				name := "rbac.authorization.k8s.io/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Namespace: &namespace,
			Name:      &name,
		},
		Subjects: Subjects,
		RoleRef:  RoleRef,
	}, metav1.ApplyOptions{FieldManager: "kok"})
}
