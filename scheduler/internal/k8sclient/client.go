package k8sclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Default resource settings
const (
	DefaultCPU    = "0.1"
	DefaultMemory = "128Mi"
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

// Ensure K8sClient implements K8sClientInterface
var _ K8sClientInterface = (*K8sClient)(nil)

// NewK8sClient creates a new K8s client
func NewK8sClient(cfg *config.Config, logger *zap.Logger) (*K8sClient, error) {
	var config *rest.Config
	var err error
	var namespace string

	// Check if we're running in a cluster
	inCluster := IsRunningInCluster()
	if inCluster {
		logger.Info("Detected in-cluster environment, using in-cluster configuration")
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load in-cluster config: %v", err)
		}

		// Get the namespace from the service account
		namespace, err = GetInClusterNamespace()
		if err != nil {
			logger.Warn("Failed to get in-cluster namespace, using configured namespace",
				zap.Error(err),
				zap.String("configured_namespace", cfg.Kubernetes.Namespace))
			namespace = cfg.Kubernetes.Namespace
		} else {
			logger.Info("Using in-cluster namespace", zap.String("namespace", namespace))
		}
	} else {
		logger.Info("Not running in cluster, using kubeconfig file")
		config, err = clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %v", err)
		}
		namespace = cfg.Kubernetes.Namespace
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
		namespace:    namespace,
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
func (k *K8sClient) ScheduleSandbox(ctx context.Context) (types.SandboxID, error) {
	namespace := k.namespace
	sandboxID := types.GenerateSandboxID()
	// Create standard labels
	labels := map[string]string{
		"app":        "sandbox",
		"managed-by": "scheduler",
		"sandbox-id": sandboxID.String(),
	}
	annotations := map[string]string{}

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
			Value: sandboxID.String(),
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
		PeriodSeconds:       1,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    5,
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
			Name:        sandboxID.WithPrefix(),
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: podSpec,
	}

	// Create the pod in Kubernetes
	_, err := k.clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create pod: %v", err)
	}

	return sandboxID, nil
}

// ReleaseSandbox deletes a sandbox pod
func (k *K8sClient) ReleaseSandbox(ctx context.Context, sandboxID types.SandboxID) error {
	namespace := k.namespace

	k.logger.Info("Deleting pod",
		sandboxID.ZapField(),
		zap.String("namespace", namespace))

	// Delete options (can be adjusted as needed)
	deleteOptions := metav1.DeleteOptions{}

	// Delete the pod
	err := k.clientset.CoreV1().Pods(namespace).Delete(ctx, sandboxID.WithPrefix(), deleteOptions)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
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
	k.watchCancel()
	// Close the event channel
	close(k.eventChan)
}

// watchNamespacePods watches for all pod events in the namespace
func (k *K8sClient) watchNamespacePods() {
	k.logger.Info("Watching pods", zap.String("namespace", k.namespace))

	// Create a watcher for all pods in the namespace
	watcher, err := k.clientset.CoreV1().Pods(k.namespace).Watch(k.watchCtx, metav1.ListOptions{
		LabelSelector: "managed-by=scheduler",
	})
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
				// Process the pod event
				k.processPodEvent(pod)
			}
		}
	}

	k.logger.Info("Pod watcher exited", zap.String("namespace", k.namespace))
}

// processPodEvent checks if a pod event is significant and sends it to the event channel if needed
func (k *K8sClient) processPodEvent(pod *corev1.Pod) {
	// Always check scheduler managed pods
	if pod.Labels["managed-by"] != "scheduler" {
		return
	}
	sandboxID, err := types.NewSandboxID(pod.Name)
	if err != nil {
		k.logger.Error("failed to parse sandbox ID", zap.Error(err))
		return
	}

	// Only process events for pods that are failed or unschedulable
	shouldSendEvent := false

	// Check for pods in failed state
	if pod.Status.Phase == corev1.PodFailed {
		shouldSendEvent = true
	} else if pod.Status.Phase == corev1.PodSucceeded {
		shouldSendEvent = true
	}

	// Check for unschedulable pods
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled &&
			condition.Status == corev1.ConditionFalse &&
			condition.Reason == "Unschedulable" {
			shouldSendEvent = true
			break
		}
	}

	// Check for container issues
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Waiting != nil {
			waitState := containerStatus.State.Waiting
			// Consider certain waiting reasons as failure events
			switch waitState.Reason {
			case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerError":
				shouldSendEvent = true
				break
			}
		}
	}

	// Only send events for pods that need attention
	if shouldSendEvent {
		event := types.PodEvent{
			SandboxID: sandboxID,
		}

		// Determine if the pod is already being deleted or should be deleted
		if pod.DeletionTimestamp != nil {
			event.EventType = types.PodAlreadyDeleted
		} else {
			event.EventType = types.PodToBeDeleted
		}

		// Send the event
		select {
		case k.eventChan <- event:
			// Event sent successfully
			k.logger.Debug("Pod event sent",
				sandboxID.ZapField(),
				zap.String("eventType", string(event.EventType)))
		default:
			// If channel is full, log a warning but don't block
			k.logger.Warn("Event channel full, dropping pod event",
				sandboxID.ZapField(),
				zap.String("eventType", string(event.EventType)))
		}
	}
}

// CreateOrUpdateProjectService creates or updates a headless service for a project
func (k *K8sClient) CreateOrUpdateProjectService(ctx context.Context, projectID types.ProjectID, sandboxID types.SandboxID) error {
	// Create the service specification
	serviceName := projectID.WithPrefix()
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: k.namespace,
			Labels: map[string]string{
				"app":        "project-service",
				"managed-by": "scheduler",
				"project-id": projectID.String(),
			},
			Annotations: map[string]string{
				"sandbox.scheduler/project-id": projectID.String(),
				"sandbox.scheduler/sandbox-id": sandboxID.String(),
			},
		},
		Spec: corev1.ServiceSpec{
			// Make it a headless service
			ClusterIP: "None",
			// Select pods with matching sandbox ID
			Selector: map[string]string{
				"sandbox-id": sandboxID.String(),
			},
			// Expose the main container port
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8000,
					TargetPort: intstr.FromInt(8000),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Try to get existing service
	existingService, err := k.clientset.CoreV1().Services(k.namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		// Service doesn't exist, create it
		_, err = k.clientset.CoreV1().Services(k.namespace).Create(ctx, service, metav1.CreateOptions{})
		if err != nil {
			k.logger.Error("failed to create project service",
				projectID.ZapField(),
				sandboxID.ZapField(),
				zap.Error(err))
			return fmt.Errorf("failed to create project service: %v", err)
		}
		k.logger.Info("Created project service",
			projectID.ZapField(),
			sandboxID.ZapField())
		return nil
	}

	// Service exists, update it
	existingService.Spec.Selector = service.Spec.Selector
	existingService.Annotations = service.Annotations
	_, err = k.clientset.CoreV1().Services(k.namespace).Update(ctx, existingService, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update project service: %v", err)
	}

	k.logger.Info("Updated project service",
		projectID.ZapField(),
		sandboxID.ZapField())
	return nil
}

// DeleteProjectService deletes a project's headless service
func (k *K8sClient) DeleteProjectService(ctx context.Context, projectID types.ProjectID) error {
	serviceName := projectID.WithPrefix()

	err := k.clientset.CoreV1().Services(k.namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete project service: %v", err)
	}

	k.logger.Info("Deleted project service",
		projectID.ZapField())
	return nil
}

// GetProjectServiceHostname returns the DNS hostname for a project's service
func (k *K8sClient) GetProjectServiceHostname(projectID types.ProjectID) string {
	return fmt.Sprintf("%s.%s.svc", projectID.WithPrefix(), k.namespace)
}

// IsRunningInCluster returns true if the application is running inside a Kubernetes cluster
func IsRunningInCluster() bool {
	// Check for the existence of the service account token
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return false
	}

	// Check for the existence of the service account CA cert
	_, err = os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return false
	}

	// Check for the existence of the service account namespace
	_, err = os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	return err == nil
}

// GetCurrentNamespace returns the namespace the client is currently using
func (k *K8sClient) GetCurrentNamespace() string {
	return k.namespace
}

// GetInClusterNamespace returns the namespace of the pod when running in-cluster
func GetInClusterNamespace() (string, error) {
	// Read the namespace from the service account
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", fmt.Errorf("failed to read in-cluster namespace: %v", err)
	}

	// Trim any whitespace
	namespace := strings.TrimSpace(string(data))
	if namespace == "" {
		return "", fmt.Errorf("in-cluster namespace is empty")
	}

	return namespace, nil
}

// IsInCluster returns true if the client is running inside a Kubernetes cluster
func (k *K8sClient) IsInCluster() bool {
	return IsRunningInCluster()
}

// IsSandboxReady checks if a sandbox pod is ready or at least scheduled
func (k *K8sClient) IsSandboxReady(ctx context.Context, sandboxID types.SandboxID) (bool, error) {
	namespace := k.namespace

	k.logger.Debug("Checking if sandbox is ready",
		sandboxID.ZapField(),
		zap.String("namespace", namespace))

	// Get the pod
	pod, err := k.clientset.CoreV1().Pods(namespace).Get(ctx, sandboxID.WithPrefix(), metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Pod doesn't exist yet
			return false, nil
		}
		return false, fmt.Errorf("failed to get pod %s: %v", sandboxID, err)
	}

	// Check if the pod is in a terminal state
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		return false, nil
	}

	// Check if the pod is scheduled
	if pod.Status.Phase == corev1.PodPending {
		// Check if the pod has been assigned to a node
		if pod.Spec.NodeName != "" {
			// Pod is scheduled but not yet running
			return false, nil
		}
		// Pod is still waiting to be scheduled
		return false, nil
	}

	// Check if the pod is running
	if pod.Status.Phase == corev1.PodRunning {
		// Check if all containers are ready
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if !containerStatus.Ready {
				return false, nil
			}
		}
		// All containers are ready
		return true, nil
	}

	// For any other state, consider it not ready
	return false, nil
}

// IsSandboxGone checks if a sandbox pod is gone (deleted or not found)
func (k *K8sClient) IsSandboxGone(ctx context.Context, sandboxID types.SandboxID) (bool, error) {
	namespace := k.namespace
	// Get the pod
	_, err := k.clientset.CoreV1().Pods(namespace).Get(ctx, sandboxID.WithPrefix(), metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Pod doesn't exist, it's gone
			return true, nil
		}
		return false, fmt.Errorf("failed to get pod %s: %v", sandboxID.WithPrefix(), err)
	}

	// Pod exists, it's not gone
	return false, nil
}

// WaitForSandboxReady waits for a sandbox pod to be ready, with a timeout
func (k *K8sClient) WaitForSandboxReady(ctx context.Context, sandboxID types.SandboxID) (bool, error) {
	namespace := k.namespace

	k.logger.Info("Waiting for sandbox to be ready",
		sandboxID.ZapField(),
		zap.String("namespace", namespace))
	// Check first before polling
	ready, err := k.IsSandboxReady(ctx, sandboxID)
	if err != nil {
		return false, fmt.Errorf("failed to check if sandbox is ready: %v", err)
	}
	if ready {
		return true, nil
	}

	// Create a ticker for polling
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Poll until the sandbox is ready or the context is done
	for {
		select {
		case <-ctx.Done():
			k.logger.Warn("Context cancelled while waiting for sandbox to be ready",
				sandboxID.ZapField())
			return false, fmt.Errorf("context cancelled while waiting for sandbox to be ready: %v", ctx.Err())
		case <-ticker.C:
			// Check if the sandbox is ready
			ready, err := k.IsSandboxReady(ctx, sandboxID)
			if err != nil {
				k.logger.Warn("Error checking if sandbox is ready",
					sandboxID.ZapField(),
					zap.Error(err))
				// Continue polling
				continue
			}

			if ready {
				k.logger.Info("Sandbox is ready",
					sandboxID.ZapField())
				return true, nil
			}

			// Check if the sandbox is gone
			gone, err := k.IsSandboxGone(ctx, sandboxID)
			if err != nil {
				k.logger.Warn("Error checking if sandbox is gone",
					sandboxID.ZapField(),
					zap.Error(err))
				// Continue polling
				continue
			}

			if gone {
				k.logger.Warn("Sandbox is gone while waiting for it to be ready",
					sandboxID.ZapField())
				return false, fmt.Errorf("sandbox is gone while waiting for it to be ready")
			}

			// Sandbox is not ready yet, continue polling
			k.logger.Debug("Sandbox is not ready yet, continuing to wait",
				sandboxID.ZapField())
		}
	}
}

// GetPodsOlderThan returns a list of pod names that were created before the specified time
// Returns pod names and a continuation token for pagination (empty if no more results)
func (k *K8sClient) GetPodsOlderThan(ctx context.Context, olderThan time.Time, continueToken string) ([]string, string, error) {
	// Fixed limit of 100 pods per page
	const limit = 100

	// Create options for listing pods
	listOptions := metav1.ListOptions{
		LabelSelector: "managed-by=scheduler",
		Limit:         int64(limit),
	}

	// Use the continue token if provided
	if continueToken != "" {
		listOptions.Continue = continueToken
	}

	// Get pods in the namespace
	pods, err := k.clientset.CoreV1().Pods(k.namespace).List(ctx, listOptions)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list pods: %v", err)
	}

	// Filter pods older than the specified time
	var olderPods []string
	for _, pod := range pods.Items {
		// Check if the pod creation time is older than the specified time
		if pod.CreationTimestamp.Time.Before(olderThan) {
			olderPods = append(olderPods, pod.Name)
		}
	}

	k.logger.Debug("Found pods older than specified time",
		zap.Time("olderThan", olderThan),
		zap.Int("foundCount", len(olderPods)),
		zap.Int("totalReturned", len(pods.Items)),
		zap.String("continueToken", pods.Continue))

	return olderPods, pods.Continue, nil
}
