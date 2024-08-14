package control

import v1 "k8s.io/client-go/applyconfigurations/rbac/v1"

func (c Kc) EventExporter() {
	sa, _ := c.ServiceAccount().Apply("kube-system", "event-exporter")
	role, _ := c.ClusterRoles().Apply("application:control-plane:event-exporter", []v1.PolicyRuleApplyConfiguration{
		{
			Verbs:     []string{"get", "watch", "list"},
			APIGroups: []string{"*"},
			Resources: []string{"*"},
		},
	})
	c.ClusterRoleBindings().Apply("application:control-plane:event-exporter", []v1.SubjectApplyConfiguration{{
		Kind: func() *string {
			v := "ServiceAccount"
			return &v
		}(),
		Namespace: func() *string {
			v := "kube-system"
			return &v
		}(),
		Name: &sa.Name},
	}, &v1.RoleRefApplyConfiguration{
		APIGroup: func() *string {
			v := "rbac.authorization.k8s.io"
			return &v
		}(),
		Kind: func() *string {
			v := "ClusterRole"
			return &v
		}(),
		Name: &role.Name,
	})
}
