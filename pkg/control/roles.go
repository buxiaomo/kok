package control

import (
	"context"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	applyrbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) Roles() roles {
	return roles{
		ck.clientset,
		context.TODO(),
	}
}

type roles struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t roles) Delete(namespace, name string) error {
	return t.clientset.RbacV1().Roles(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (t roles) Apply(namespace, name string, rules []applyrbacv1.PolicyRuleApplyConfiguration) (result *v1.Role, err error) {
	return t.clientset.RbacV1().Roles(namespace).Apply(t.ctx, &applyrbacv1.RoleApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "Role"
				return &name
			}(),
			APIVersion: func() *string {
				name := "rbac.authorization.k8s.io/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name:      &name,
			Namespace: &namespace,
		},
		Rules: rules,
	}, metav1.ApplyOptions{FieldManager: "kok"})
}
