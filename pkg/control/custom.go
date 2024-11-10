package control

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	v1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/klog/v2"
	"strconv"
	"time"
)

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

func (c Kc) ServerVersion() (*version.Info, error) {
	return c.clientset.Discovery().ServerVersion()
}

func (c Kc) isStatefulSetFullyUpdated(namespace, name string) (bool, error) {
	statefulSet, err := c.clientset.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if statefulSet.Status.UpdatedReplicas != *statefulSet.Spec.Replicas ||
		statefulSet.Status.ReadyReplicas != *statefulSet.Spec.Replicas ||
		statefulSet.Status.CurrentReplicas != *statefulSet.Spec.Replicas ||
		statefulSet.Status.ObservedGeneration < statefulSet.Generation {
		return false, nil
	}
	labelSelector := metav1.FormatLabelSelector(statefulSet.Spec.Selector)
	pods, err := c.clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return false, err
	}
	for _, pod := range pods.Items {
		// 检查每个 Pod 的 OwnerReference 是否为当前的 StatefulSet
		isCurrent := false
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "StatefulSet" && ownerRef.Name == statefulSet.Name {
				isCurrent = true
				break
			}
		}
		if !isCurrent || pod.Status.Phase == corev1.PodFailed || pod.DeletionTimestamp != nil {
			return false, nil // 仍存在旧 Pod，尚未完全更新
		}
	}
	return true, nil
}

func (c Kc) WaitForStatefulSetUpdate(namespace, name string) (err error) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeoutChan := time.After(3 * time.Minute)
	for {
		select {
		case <-ticker.C:
			updated, err := c.isStatefulSetFullyUpdated(namespace, name)
			if err != nil {
				return err
			}
			if updated {
				fmt.Printf("StatefulSet %s has been successfully updated\n", name)
				return nil
			}
		case <-timeoutChan:
			return fmt.Errorf("timed out waiting for StatefulSet %s to update", name)
		}
	}
}

func (c Kc) getLatestReplicaSetName(deployment *appsv1.Deployment) (*appsv1.ReplicaSet, error) {
	rsList, err := c.clientset.AppsV1().ReplicaSets(deployment.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
	})
	if err != nil {
		return nil, err
	}

	// 用于记录最新的 ReplicaSet
	var latestRS *appsv1.ReplicaSet
	maxRevision := int64(-1)

	for _, rs := range rsList.Items {
		// 检查该 ReplicaSet 是否属于当前 Deployment
		for _, ownerRef := range rs.OwnerReferences {
			if ownerRef.Kind == "Deployment" && ownerRef.Name == deployment.Name {
				// 解析 revision 标签
				revisionStr, ok := rs.Annotations["deployment.kubernetes.io/revision"]
				if !ok {
					continue
				}
				revision, err := strconv.ParseInt(revisionStr, 10, 64)
				if err != nil {
					klog.Infof("Failed to parse revision for ReplicaSet %s: %v", rs.Name, err)
					continue
				}
				// 更新最新 ReplicaSet
				if revision > maxRevision {
					maxRevision = revision
					latestRS = &rs
				}
			}
		}
	}

	if latestRS == nil {
		return nil, fmt.Errorf("no ReplicaSet found for Deployment %s", deployment.Name)
	}

	return latestRS, nil
}

func (c Kc) isDeploymentFullyUpdated(namespace string, deployment *appsv1.Deployment) error {
	// 获取最新的 ReplicaSet 名称
	rs, err := c.getLatestReplicaSetName(deployment)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeoutChan := time.After(3 * time.Minute)

	for {
		select {
		case <-ticker.C:
			pods, err := c.clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
				LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
			})
			if err != nil {
				return err
			}
			var isOK []bool

			for _, pod := range pods.Items {
				for _, ownerRef := range pod.OwnerReferences {
					//fmt.Println(ownerRef.Name, rs.Name)
					if ownerRef.Name == rs.Name {
						//isCurrent = true
						isOK = append(isOK, true)
					} else {
						isOK = append(isOK, false)
					}
				}
			}

			if len(isOK) == 1 && isOK[0] == true {
				return nil
			}

		case <-timeoutChan:
			return fmt.Errorf("timed out waiting for Deployment %s to update", deployment.Name)
		}
	}
}

func (c Kc) WaitForDeploymentUpdate(namespace string, deployment *appsv1.Deployment) (err error) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeoutChan := time.After(3 * time.Minute)
	for {
		select {
		case <-ticker.C:
			err := c.isDeploymentFullyUpdated(namespace, deployment)
			if err != nil {
				return err
			} else {
				fmt.Printf("Deployment %s has been successfully updated\n", deployment.Name)
				return nil
			}
		case <-timeoutChan:
			return fmt.Errorf("timed out waiting for Deployment %s to update", deployment.Name)
		}
	}
}
