package control

import (
	"context"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	applyrbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) ClusterRoleBindings() clusterrolebindings {
	return clusterrolebindings{
		ck.clientset,
		context.TODO(),
	}
}

type clusterrolebindings struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t clusterrolebindings) Delete(name string) error {
	return t.clientset.RbacV1().ClusterRoleBindings().Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (t clusterrolebindings) Apply(name string, Subjects []applyrbacv1.SubjectApplyConfiguration, RoleRef *applyrbacv1.RoleRefApplyConfiguration) (result *v1.ClusterRoleBinding, err error) {
	return t.clientset.RbacV1().ClusterRoleBindings().Apply(t.ctx, &applyrbacv1.ClusterRoleBindingApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				v := "ClusterRoleBinding"
				return &v
			}(),
			APIVersion: func() *string {
				v := "rbac.authorization.k8s.io/v1"
				return &v
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: &name,
		},
		Subjects: Subjects,
		RoleRef:  RoleRef,
	}, metav1.ApplyOptions{FieldManager: "kok"})
}
