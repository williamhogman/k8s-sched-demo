package k8sclient

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Default resource settings
const (
	DefaultCPU    = "0.5"
	DefaultMemory = "512Mi"
)

// K8sClient is a wrapper around the Kubernetes client
type K8sClient struct {
	clientset *kubernetes.Clientset
}

// NewK8sClient creates a new K8s client
func NewK8sClient() (*K8sClient, error) {
	// Get kubeconfig from default location
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %v", err)
	}

	// Creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	return &K8sClient{
		clientset: clientset,
	}, nil
}

// NewTestK8sClient creates a new K8s client for testing with explicit config
func NewTestK8sClient(config *rest.Config) (*K8sClient, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	return &K8sClient{
		clientset: clientset,
	}, nil
}

// ScheduleSandbox creates a pod for the sandbox
func (k *K8sClient) ScheduleSandbox(ctx context.Context, podName, namespace string, metadata map[string]string) (string, error) {
	// Create pod labels
	labels := map[string]string{
		"app": "sandbox",
	}

	// Add metadata as labels
	for key, value := range metadata {
		if key != "" && value != "" {
			labels[fmt.Sprintf("sandbox-metadata-%s", key)] = value
		}
	}

	// Create resource requirements with default values
	resourceRequirements := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(DefaultCPU),
			corev1.ResourceMemory: resource.MustParse(DefaultMemory),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(DefaultCPU),
			corev1.ResourceMemory: resource.MustParse(DefaultMemory),
		},
	}

	// Create the pod object
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:      "sandbox-container",
					Image:     getImageFromMetadata(metadata),
					Resources: resourceRequirements,
				},
			},
		},
	}

	// Create the pod
	createdPod, err := k.clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create pod: %v", err)
	}

	// Watch pod until it's scheduled
	go k.watchPodStatus(namespace, createdPod.Name)

	return createdPod.Name, nil
}

// ReleaseSandbox deletes a sandbox pod
func (k *K8sClient) ReleaseSandbox(ctx context.Context, sandboxID string) error {
	// Check if the sandbox ID starts with "sandbox-" prefix, if not, prepend it
	podName := sandboxID
	if len(podName) > 8 && podName[:8] != "sandbox-" {
		podName = fmt.Sprintf("sandbox-%s", sandboxID)
	}

	// Default namespace if not specified in the ID
	namespace := "default"

	// If sandboxID contains namespace info (in format namespace/podName), extract it
	if parts := strings.Split(podName, "/"); len(parts) == 2 {
		namespace = parts[0]
		podName = parts[1]
	}

	log.Printf("Deleting pod %s in namespace %s", podName, namespace)

	// Delete options (can be adjusted as needed)
	deleteOptions := metav1.DeleteOptions{}

	// Delete the pod
	err := k.clientset.CoreV1().Pods(namespace).Delete(ctx, podName, deleteOptions)
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %v", podName, err)
	}

	return nil
}

// watchPodStatus watches the pod status and logs events (simplified monitoring)
func (k *K8sClient) watchPodStatus(namespace, podName string) {
	for {
		pod, err := k.clientset.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Error getting pod status: %v", err)
			return
		}

		log.Printf("Pod %s status: %s", podName, pod.Status.Phase)

		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return
		}

		time.Sleep(5 * time.Second)
	}
}

// Helper function to get image from metadata
func getImageFromMetadata(metadata map[string]string) string {
	if image, ok := metadata["image"]; ok && image != "" {
		return image
	}
	return "busybox:latest" // Default image
}
