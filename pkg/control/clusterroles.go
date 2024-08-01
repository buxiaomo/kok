package control

import (
	"context"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	applyrbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) ClusterRoles() clusterroles {
	return clusterroles{
		ck.clientset,
		context.TODO(),
	}
}

type clusterroles struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t clusterroles) Delete(name string) error {
	return t.clientset.RbacV1().ClusterRoles().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (t clusterroles) Apply(name string, rules []applyrbacv1.PolicyRuleApplyConfiguration) (result *v1.ClusterRole, err error) {
	return t.clientset.RbacV1().ClusterRoles().Apply(t.ctx, &applyrbacv1.ClusterRoleApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "ClusterRole"
				return &name
			}(),
			APIVersion: func() *string {
				name := "rbac.authorization.k8s.io/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: &name,
		},
		Rules: rules,
	}, metav1.ApplyOptions{FieldManager: "kok"})
}
