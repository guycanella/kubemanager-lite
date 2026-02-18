package kubernetes

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodInfo is the struct that will be serialized automatically by Wails
// in TypeScript via binding. All exported fields become TS properties.
// We keep only the relevant fields for the dashboard.
type PodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`   // Running, Pending, Failed, Succeeded, Unknown
	Ready     bool   `json:"ready"`    // true if all containers are Ready
	Restarts  int32  `json:"restarts"` // total restarts (sum of all containers)
	NodeName  string `json:"nodeName"`
	Age       int64  `json:"age"`   // Unix timestamp of creation
	Image     string `json:"image"` // image of the first container (simplification for MVP)
}

// ListPods returns all pods of a namespace.
// If namespace is "" (empty), lists all namespaces.
func (c *Client) ListPods(ctx context.Context, namespace string) ([]PodInfo, error) {
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing pods in namespace '%s': %w", namespace, err)
	}

	result := make([]PodInfo, 0, len(podList.Items))
	for _, pod := range podList.Items {
		result = append(result, toPodInfo(pod))
	}

	return result, nil
}

// ListNamespaces returns all available namespaces in the cluster.
// Used in the frontend to populate the namespace selector.
func (c *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	nsList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing namespaces: %w", err)
	}

	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}

	return names, nil
}

// --- Helpers internos ---

// toPodInfo converts a Kubernetes Pod to our simplified PodInfo.
func toPodInfo(pod corev1.Pod) PodInfo {
	info := PodInfo{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Status:    string(pod.Status.Phase),
		NodeName:  pod.Spec.NodeName,
	}

	// Creation date
	if pod.CreationTimestamp.Time.IsZero() == false {
		info.Age = pod.CreationTimestamp.Time.Unix()
	}

	// Image of the first container
	if len(pod.Spec.Containers) > 0 {
		info.Image = pod.Spec.Containers[0].Image
	}

	// Sum restarts and check if all containers are Ready
	readyCount := 0
	for _, cs := range pod.Status.ContainerStatuses {
		info.Restarts += cs.RestartCount
		if cs.Ready {
			readyCount++
		}
	}

	info.Ready = len(pod.Spec.Containers) > 0 &&
		readyCount == len(pod.Spec.Containers)

	return info
}
