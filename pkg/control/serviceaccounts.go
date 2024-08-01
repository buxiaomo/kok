package control

import (
	"context"
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type serviceaccounts struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t serviceaccounts) CreateToken(namespace, name string, ExpirationSeconds int64) (*authenticationv1.TokenRequest, error) {
	return t.clientset.CoreV1().ServiceAccounts(namespace).CreateToken(t.ctx, name, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &ExpirationSeconds,
		},
	}, metav1.CreateOptions{})
}

func (t serviceaccounts) Get(namespace, name string) (*v1.ServiceAccount, error) {
	return t.clientset.CoreV1().ServiceAccounts(namespace).Get(t.ctx, name, metav1.GetOptions{})
}

func (t serviceaccounts) Delete(namespace, name string) error {
	return t.clientset.CoreV1().ServiceAccounts(namespace).Delete(t.ctx, name, metav1.DeleteOptions{})
}

func (t serviceaccounts) Apply(namespace, name string) (result *v1.ServiceAccount, err error) {
	return t.clientset.CoreV1().ServiceAccounts(namespace).Apply(t.ctx, &applycorev1.ServiceAccountApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "ServiceAccount"
				return &name
			}(),
			APIVersion: func() *string {
				name := "v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name:      &name,
			Namespace: &namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
}

func (ck Kc) ServiceAccount() serviceaccounts {
	return serviceaccounts{
		ck.clientset,
		context.TODO(),
	}
}
