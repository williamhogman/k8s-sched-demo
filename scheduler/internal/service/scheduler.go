package service

import (
	"context"
	"fmt"
	"log"
)

// K8sClientInterface defines the interface for Kubernetes client
type K8sClientInterface interface {
	ScheduleSandbox(ctx context.Context, sandboxID, namespace string, config map[string]string) (string, error)
}

// SchedulerService implements the scheduling service logic
type SchedulerService struct {
	k8sClient K8sClientInterface
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(k8sClient K8sClientInterface) *SchedulerService {
	return &SchedulerService{
		k8sClient: k8sClient,
	}
}

// ScheduleSandbox handles scheduling a sandbox
func (s *SchedulerService) ScheduleSandbox(ctx context.Context, sandboxID, namespace string, config map[string]string) (string, error) {
	if sandboxID == "" {
		return "", fmt.Errorf("sandbox ID cannot be empty")
	}

	if namespace == "" {
		namespace = "default" // Use default namespace if not specified
	}

	log.Printf("Scheduling sandbox %s in namespace %s", sandboxID, namespace)

	// Schedule the sandbox on Kubernetes
	podName, err := s.k8sClient.ScheduleSandbox(ctx, sandboxID, namespace, config)
	if err != nil {
		log.Printf("Failed to schedule sandbox %s: %v", sandboxID, err)
		return "", fmt.Errorf("failed to schedule: %v", err)
	}

	log.Printf("Successfully scheduled sandbox %s as pod %s", sandboxID, podName)
	return podName, nil
}
