package k8sclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
	"go.uber.org/zap"
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
	useGvisor    bool   // Whether to enforce gVisor runtime
	sandboxImage string // The sandbox image to use
	// Context for controlling the watcher lifecycle
	watchCtx    context.Context
	watchCancel context.CancelFunc
	// Logger
	logger *zap.Logger
	// Event channel for pod events
	eventChan chan types.PodEvent
}

// NewK8sClient creates a new K8s client
func NewK8sClient(cfg *config.Config, logger *zap.Logger) (*K8sClient, error) {
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
		clientset:    clientset,
		namespace:    cfg.Kubernetes.Namespace,
		useGvisor:    cfg.Kubernetes.UseGvisor,
		sandboxImage: cfg.Kubernetes.SandboxImage,
		watchCtx:     ctx,
		watchCancel:  cancel,
		logger:       logger.Named("k8sclient"),
		eventChan:    make(chan types.PodEvent, 100), // Buffer to avoid blocking
	}

	return client, nil
}

// NewTestK8sClient creates a new K8s client for testing with explicit config
func NewTestK8sClient(config *rest.Config, logger *zap.Logger) (*K8sClient, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	// Create a context with cancel function for the watchers
	ctx, cancel := context.WithCancel(context.Background())

	return &K8sClient{
		clientset:    clientset,
		namespace:    "sandbox",
		useGvisor:    false,            // Default to false for testing
		sandboxImage: "sandbox:latest", // Default image for testing
		watchCtx:     ctx,
		watchCancel:  cancel,
		logger:       logger.Named("k8sclient"),
		eventChan:    make(chan types.PodEvent, 100), // Buffer to avoid blocking
	}, nil
}

// GetEventChannel returns the channel for pod events
func (k *K8sClient) GetEventChannel() <-chan types.PodEvent {
	return k.eventChan
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
		Image:           k.sandboxImage,
		Args:            []string{},
		ImagePullPolicy: corev1.PullIfNotPresent,
		Resources:       resourceRequirements,
		Env:             envVars,
	}

	// Configure liveness probe for health checking
	container.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromInt(8000),
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
				Path: "/ready",
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
	namespace := k.namespace

	k.logger.Info("Deleting pod",
		zap.String("pod", sandboxID),
		zap.String("namespace", namespace))

	// Delete options (can be adjusted as needed)
	deleteOptions := metav1.DeleteOptions{}

	// Delete the pod
	err := k.clientset.CoreV1().Pods(namespace).Delete(ctx, sandboxID, deleteOptions)
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %v", sandboxID, err)
	}

	return nil
}

// StartWatchers initiates watching for pod events in the namespace
func (k *K8sClient) StartWatchers() {
	k.logger.Info("Starting pod watcher", zap.String("namespace", k.namespace))
	go k.watchNamespacePods()
}

// StopWatchers cancels all running watchers
func (k *K8sClient) StopWatchers() {
	k.logger.Info("Stopping pod watchers")
	k.watchCancel()
	// Close the event channel
	close(k.eventChan)
}

// watchNamespacePods watches for all pod events in the namespace
func (k *K8sClient) watchNamespacePods() {
	k.logger.Info("Watching pods", zap.String("namespace", k.namespace))

	// Create a watcher for all pods in the namespace
	watcher, err := k.clientset.CoreV1().Pods(k.namespace).Watch(k.watchCtx, metav1.ListOptions{})
	if err != nil {
		k.logger.Error("Error creating pod watcher", zap.Error(err))
		return
	}
	defer watcher.Stop()

	// Process pod events
	for event := range watcher.ResultChan() {
		select {
		case <-k.watchCtx.Done():
			k.logger.Info("Pod watcher context cancelled, exiting")
			return
		default:
			if pod, ok := event.Object.(*corev1.Pod); ok {
				// Check if this pod is managed by our scheduler
				if pod.Labels["managed-by"] == "scheduler" {
					// Process the pod event to determine if we need to send a notification
					k.processPodEvent(pod)
				}
			}
		}
	}

	k.logger.Info("Pod watcher exited", zap.String("namespace", k.namespace))
}

// processPodEvent checks if a pod event is significant and sends it to the event channel if needed
func (k *K8sClient) processPodEvent(pod *corev1.Pod) {
	var eventType types.PodEventType
	var reason, message string
	shouldSendEvent := false

	// Check the pod phase
	switch pod.Status.Phase {
	case corev1.PodFailed:
		eventType = types.PodEventFailed
		reason = "PodFailed"
		message = "Pod entered Failed phase"
		shouldSendEvent = true
	case corev1.PodSucceeded:
		eventType = types.PodEventSucceeded
		reason = "PodSucceeded"
		message = "Pod completed successfully"
		shouldSendEvent = true
	}

	// Check for unschedulable condition
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
			if condition.Reason == "Unschedulable" {
				eventType = types.PodEventUnschedulable
				reason = condition.Reason
				message = condition.Message
				shouldSendEvent = true
				break
			}
		}
	}

	// If containers are in a terminal state, check for reasons
	for _, containerStatus := range pod.Status.ContainerStatuses {
		// Check for waiting containers with issues
		if containerStatus.State.Waiting != nil {
			waitState := containerStatus.State.Waiting

			// Consider certain waiting reasons as failure events
			switch waitState.Reason {
			case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerError":
				eventType = types.PodEventFailed
				reason = fmt.Sprintf("ContainerWaiting:%s", waitState.Reason)
				message = fmt.Sprintf("Container %s is waiting: %s",
					containerStatus.Name, waitState.Message)
				shouldSendEvent = true
			}
		}
	}

	// Send the event if needed
	if shouldSendEvent {
		event := types.PodEvent{
			PodName:   pod.Name,
			EventType: eventType,
			Reason:    reason,
			Message:   message,
			Timestamp: time.Now(),
		}

		select {
		case k.eventChan <- event:
			k.logger.Info("Sent pod event to channel",
				zap.String("pod", pod.Name),
				zap.String("eventType", string(eventType)),
				zap.String("reason", reason))
		default:
			// If channel is full, log a warning but don't block
			k.logger.Warn("Event channel full, dropping pod event",
				zap.String("pod", pod.Name),
				zap.String("eventType", string(eventType)))
		}
	}
}
