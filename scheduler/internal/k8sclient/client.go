package k8sclient

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	namespace string
	// Configuration options
	useGvisor bool // Whether to enforce gVisor runtime
	// Context for controlling the watcher lifecycle
	watchCtx    context.Context
	watchCancel context.CancelFunc
}

// NewK8sClient creates a new K8s client
func NewK8sClient(cfg *config.Config) (*K8sClient, error) {
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

	// Create a context with cancel function for the watchers
	ctx, cancel := context.WithCancel(context.Background())

	client := &K8sClient{
		clientset:   clientset,
		namespace:   cfg.Kubernetes.Namespace,
		useGvisor:   cfg.Kubernetes.UseGvisor,
		watchCtx:    ctx,
		watchCancel: cancel,
	}

	return client, nil
}

// NewTestK8sClient creates a new K8s client for testing with explicit config
func NewTestK8sClient(config *rest.Config) (*K8sClient, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	// Create a context with cancel function for the watchers
	ctx, cancel := context.WithCancel(context.Background())

	return &K8sClient{
		clientset:   clientset,
		namespace:   "sandbox",
		useGvisor:   false, // Default to false for testing
		watchCtx:    ctx,
		watchCancel: cancel,
	}, nil
}

// ScheduleSandbox creates a pod for the sandbox
func (k *K8sClient) ScheduleSandbox(ctx context.Context, podName string, metadata map[string]string) (string, error) {
	// Use the client's namespace
	namespace := k.namespace

	// Create standard labels
	labels := map[string]string{
		"app":        "sandbox",
		"managed-by": "scheduler",
		"sandbox-id": podName,
	}

	// Add all metadata as labels with a prefix to avoid conflicts
	for key, value := range metadata {
		if key != "" && value != "" {
			labels[fmt.Sprintf("sandbox-metadata-%s", key)] = value
		}
	}

	// Create standard annotations
	annotations := map[string]string{
		"sandbox.scheduler/scheduled":          "true",
		"sandbox.scheduler/creation-timestamp": time.Now().Format(time.RFC3339),
		"sandbox.scheduler/pod-name":           podName,
	}

	// Define resource requirements (customizable in the future)
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

	// Define environment variables with some useful defaults
	envVars := []corev1.EnvVar{
		{
			Name:  "SANDBOX_ID",
			Value: podName,
		},
		{
			Name:  "SANDBOX_NAMESPACE",
			Value: namespace,
		},
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
	}

	// Create the container specification
	container := corev1.Container{
		Name:            "sandbox-container",
		Image:           "kennethreitz/httpbin",
		Args:            []string{"gunicorn", "-b", "0.0.0.0:8000", "httpbin:app", "-k", "gevent"},
		ImagePullPolicy: corev1.PullIfNotPresent,
		Resources:       resourceRequirements,
		Env:             envVars,
	}

	// Configure liveness probe for health checking
	container.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromInt(80),
			},
		},
		InitialDelaySeconds: 1,
		PeriodSeconds:       60,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
	container.StartupProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromInt(8000),
			},
		},
		InitialDelaySeconds: 0,
		PeriodSeconds:       5,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}

	// Optional volume mounts could be added here
	// container.VolumeMounts = []corev1.VolumeMount{...}

	// Create pod specification
	podSpec := corev1.PodSpec{
		Containers:    []corev1.Container{container},
		RestartPolicy: corev1.RestartPolicyNever, // Sandboxes shouldn't restart automatically

		// Security context for the pod
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &[]bool{true}[0],
			RunAsUser:    &[]int64{1000}[0], // Run as non-root user
			FSGroup:      &[]int64{1000}[0],
		},

		// Termination Grace Period (30 seconds is the default)
		TerminationGracePeriodSeconds: &[]int64{30}[0],

		// DNS policy (ClusterFirst is the default)
		DNSPolicy: corev1.DNSNone,
		DNSConfig: &corev1.PodDNSConfig{
			Nameservers: []string{"8.8.8.8", "1.1.1.1"},
		},
	}

	// Add gVisor configuration if enabled
	if k.useGvisor {
		// Add node selector for gVisor
		if podSpec.NodeSelector == nil {
			podSpec.NodeSelector = make(map[string]string)
		}
		podSpec.NodeSelector["sandbox.gke.io/runtime"] = "gvisor"

		// Add toleration for gVisor
		gvisorToleration := corev1.Toleration{
			Key:      "sandbox.gke.io/runtime",
			Operator: corev1.TolerationOpEqual,
			Value:    "gvisor",
			Effect:   corev1.TaintEffectNoSchedule,
		}
		podSpec.Tolerations = append(podSpec.Tolerations, gvisorToleration)

		// Add annotation indicating gVisor is being used
		annotations["sandbox.scheduler/isolation"] = "gvisor"
	}

	// Create full pod specification
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: podSpec,
	}

	// Create the pod in Kubernetes
	createdPod, err := k.clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create pod: %v", err)
	}

	return createdPod.Name, nil
}

// ReleaseSandbox deletes a sandbox pod
func (k *K8sClient) ReleaseSandbox(ctx context.Context, sandboxID string) error {
	// Check if the sandbox ID starts with "sandbox-" prefix, if not, prepend it
	podName := sandboxID
	if len(podName) > 8 && podName[:8] != "sandbox-" {
		podName = fmt.Sprintf("sandbox-%s", sandboxID)
	}

	// Always use the client's namespace
	namespace := k.namespace

	// Log deletion
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

// StartWatchers initiates watching for pod events in the namespace
func (k *K8sClient) StartWatchers() {
	log.Printf("Starting pod watcher for namespace: %s", k.namespace)
	go k.watchNamespacePods()
}

// StopWatchers cancels all running watchers
func (k *K8sClient) StopWatchers() {
	log.Printf("Stopping pod watchers")
	k.watchCancel()
}

// watchNamespacePods watches for all pod events in the namespace
func (k *K8sClient) watchNamespacePods() {
	log.Printf("Watching pods in namespace %s", k.namespace)

	// Create a watcher for all pods in the namespace
	watcher, err := k.clientset.CoreV1().Pods(k.namespace).Watch(k.watchCtx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Error creating pod watcher: %v", err)
		return
	}
	defer watcher.Stop()

	// Process pod events
	for event := range watcher.ResultChan() {
		select {
		case <-k.watchCtx.Done():
			log.Printf("Pod watcher context cancelled, exiting")
			return
		default:
			if pod, ok := event.Object.(*corev1.Pod); ok {
				// Log the pod event
				log.Printf("Pod event: type=%s, name=%s, phase=%s",
					event.Type, pod.Name, pod.Status.Phase)

				// Here we could handle different types of events and pod phases
				// For now we're just logging them
			}
		}
	}

	log.Printf("Pod watcher for namespace %s exited", k.namespace)
}
