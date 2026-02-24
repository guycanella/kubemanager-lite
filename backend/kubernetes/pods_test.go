package kubernetes

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── toPodInfo ────────────────────────────────────────────────────────────────

func TestToPodInfo_BasicFields(t *testing.T) {
	createdAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nginx-abc123",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(createdAt),
		},
		Spec: corev1.PodSpec{
			NodeName: "minikube",
			Containers: []corev1.Container{
				{Image: "nginx:alpine"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	info := toPodInfo(pod)

	if info.Name != "nginx-abc123" {
		t.Errorf("Name = %q, want %q", info.Name, "nginx-abc123")
	}
	if info.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q", info.Namespace, "default")
	}
	if info.Status != "Running" {
		t.Errorf("Status = %q, want %q", info.Status, "Running")
	}
	if info.NodeName != "minikube" {
		t.Errorf("NodeName = %q, want %q", info.NodeName, "minikube")
	}
	if info.Image != "nginx:alpine" {
		t.Errorf("Image = %q, want %q", info.Image, "nginx:alpine")
	}
	if info.Age != createdAt.Unix() {
		t.Errorf("Age = %d, want %d", info.Age, createdAt.Unix())
	}
}

func TestToPodInfo_RestartCountSum(t *testing.T) {
	tests := []struct {
		name             string
		containerStats   []corev1.ContainerStatus
		expectedRestarts int32
	}{
		{
			name:             "no restarts",
			containerStats:   []corev1.ContainerStatus{{RestartCount: 0}},
			expectedRestarts: 0,
		},
		{
			name:             "single container with restarts",
			containerStats:   []corev1.ContainerStatus{{RestartCount: 5}},
			expectedRestarts: 5,
		},
		{
			name: "multiple containers — restarts are summed",
			containerStats: []corev1.ContainerStatus{
				{RestartCount: 3},
				{RestartCount: 2},
				{RestartCount: 1},
			},
			expectedRestarts: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: make([]corev1.Container, len(tt.containerStats)),
				},
				Status: corev1.PodStatus{
					ContainerStatuses: tt.containerStats,
				},
			}
			info := toPodInfo(pod)
			if info.Restarts != tt.expectedRestarts {
				t.Errorf("Restarts = %d, want %d", info.Restarts, tt.expectedRestarts)
			}
		})
	}
}

func TestToPodInfo_ReadyState(t *testing.T) {
	tests := []struct {
		name           string
		containers     []corev1.Container
		containerStats []corev1.ContainerStatus
		expectedReady  bool
	}{
		{
			name:       "all containers ready",
			containers: []corev1.Container{{}, {}},
			containerStats: []corev1.ContainerStatus{
				{Ready: true},
				{Ready: true},
			},
			expectedReady: true,
		},
		{
			name:       "one container not ready",
			containers: []corev1.Container{{}, {}},
			containerStats: []corev1.ContainerStatus{
				{Ready: true},
				{Ready: false},
			},
			expectedReady: false,
		},
		{
			name:           "no containers",
			containers:     []corev1.Container{},
			containerStats: []corev1.ContainerStatus{},
			expectedReady:  false,
		},
		{
			name:       "single container ready",
			containers: []corev1.Container{{}},
			containerStats: []corev1.ContainerStatus{
				{Ready: true},
			},
			expectedReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: tt.containers,
				},
				Status: corev1.PodStatus{
					ContainerStatuses: tt.containerStats,
				},
			}
			info := toPodInfo(pod)
			if info.Ready != tt.expectedReady {
				t.Errorf("Ready = %v, want %v", info.Ready, tt.expectedReady)
			}
		})
	}
}

func TestToPodInfo_ImageFallback(t *testing.T) {
	t.Run("uses first container image", func(t *testing.T) {
		pod := corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "first:latest"},
					{Image: "second:latest"},
				},
			},
		}
		info := toPodInfo(pod)
		if info.Image != "first:latest" {
			t.Errorf("Image = %q, want %q", info.Image, "first:latest")
		}
	})

	t.Run("empty image when no containers", func(t *testing.T) {
		pod := corev1.Pod{
			Spec: corev1.PodSpec{Containers: []corev1.Container{}},
		}
		info := toPodInfo(pod)
		if info.Image != "" {
			t.Errorf("Image = %q, want empty string", info.Image)
		}
	})
}
