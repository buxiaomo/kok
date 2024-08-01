package control

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type Kc struct {
	clientset kubernetes.Interface
}

func New(kubeconfig string) *Kc {
	config, err := createKubeconfig(kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	c, err := kubernetes.NewForConfig(config)

	if err != nil {
		panic(err.Error())
	}

	return &Kc{
		clientset: c,
	}
}

func (c Kc) HasDefaultSC() bool {
	scs, err := c.StorageClasses().List()
	if err != nil {
		panic(err.Error())
	}
	for _, sc := range scs.Items {
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return true
		}
	}
	return false
}

func (c Kc) ClearPodOnFaultyNode() error {
	w, err := c.Nodes().Watch()
	if err != nil {
		return err
	}
	ch := w.ResultChan()
	for event := range ch {
		switch event.Type {
		case watch.Added, watch.Modified:
			node := event.Object.(*corev1.Node)
			for _, condition := range node.Status.Conditions {
				if condition.Type == "Ready" && condition.Status != "True" {
					podList, err := c.Pods().List("", metav1.ListOptions{
						FieldSelector: "spec.nodeName=" + node.Name,
						LabelSelector: "app=etcd",
					})
					if err != nil {
						panic(err)
					}
					for _, pod := range podList.Items {
						ns, _ := c.Namespace().Get(pod.Namespace)
						if ns.Labels["fieldManager"] == "control-plane" {
							err := c.Pods().Delete(pod.Namespace, pod.Name)
							fmt.Printf("Node %s is NotReady, delete pod: %s in namesapce: %s, err: %v\n", node.Name, pod.Name, pod.Namespace, err)
						}
					}
				}
			}
		case watch.Deleted:
			fmt.Printf("Node deleted: %s\n", event.Object.(*corev1.Node).GetName())
		case watch.Error:
			fmt.Printf("Error: %v\n", event.Object)
		}
	}
	return err
}
