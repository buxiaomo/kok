package control

import (
	"context"
	"encoding/json"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"time"
)

type rollout struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t rollout) Restart(namespace string, tp string, name []string) {
	opt := metav1.PatchOptions{
		FieldManager: "kubectl rollout",
	}
	dt := time.Now()
	patchBytes, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"kubectl.kubernetes.io/restartedAt": dt.String(),
					},
				},
			},
		},
	})
	switch tp {
	case "deployment":
		for _, i := range name {
			t.clientset.AppsV1().Deployments(namespace).Patch(t.ctx, i, types.StrategicMergePatchType, patchBytes, opt)
		}
	case "statefulset":
		for _, i := range name {
			t.clientset.AppsV1().StatefulSets(namespace).Patch(t.ctx, i, types.StrategicMergePatchType, patchBytes, opt)
		}
	case "daemonset":
		for _, i := range name {
			t.clientset.AppsV1().DaemonSets(namespace).Patch(t.ctx, i, types.StrategicMergePatchType, patchBytes, opt)
		}
	}
}

func (ck Kc) Rollout() rollout {
	return rollout{
		ck.clientset,
		context.TODO(),
	}
}
