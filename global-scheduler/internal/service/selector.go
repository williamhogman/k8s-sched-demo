package service

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	selectorv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/global-scheduler/v1"
	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/global-scheduler/internal/client"
	"github.com/williamhogman/k8s-sched-demo/global-scheduler/internal/models"
)

// SelectorService provides implementation for the ClusterSelector service
type SelectorService struct {
	// available clusters for selection
	clusters map[string]*models.Cluster
	// client to communicate with scheduler service
	schedulerClient client.SchedulerClientInterface
}

// NewSelectorService creates a new selector service instance
func NewSelectorService(clusters map[string]*models.Cluster, schedulerClient client.SchedulerClientInterface) *SelectorService {
	return &SelectorService{
		clusters:        clusters,
		schedulerClient: schedulerClient,
	}
}

// GetSandbox implements the HTTP endpoint that handles both selecting a cluster
// and scheduling a sandbox in one call
func (s *SelectorService) GetSandbox(ctx context.Context, req *selectorv1.GetSandboxRequest) (*selectorv1.GetSandboxResponse, error) {
	// Generate a sandbox ID if one is not provided
	sandboxID := req.SandboxId
	if sandboxID == "" {
		sandboxID = fmt.Sprintf("sandbox-%d", time.Now().Unix())
	}

	// Get all available clusters
	clusters := s.clusters
	if len(clusters) == 0 {
		return &selectorv1.GetSandboxResponse{
			Status:       selectorv1.GetSandboxStatus_GET_SANDBOX_STATUS_FAILED,
			ErrorMessage: "no clusters available",
			SandboxId:    sandboxID,
		}, nil
	}

	// Convert map to slice for selection algorithm
	var clusterSlice []*models.Cluster
	for _, cluster := range clusters {
		clusterSlice = append(clusterSlice, cluster)
	}

	// Group clusters by priority (highest first)
	priorityGroups := groupClustersByPriority(clusterSlice)
	if len(priorityGroups) == 0 {
		return &selectorv1.GetSandboxResponse{
			Status:       selectorv1.GetSandboxStatus_GET_SANDBOX_STATUS_FAILED,
			ErrorMessage: "failed to group clusters by priority",
			SandboxId:    sandboxID,
		}, nil
	}

	// Get highest priority clusters
	highestPriorityGroup := priorityGroups[0].clusters

	// Select a cluster - if only one cluster in the highest priority, use it
	var selectedCluster *models.Cluster
	if len(highestPriorityGroup) == 1 {
		selectedCluster = highestPriorityGroup[0]
	} else {
		// Use weighted random selection for clusters with the same highest priority
		selectedCluster = weightedRandomSelection(highestPriorityGroup)
		if selectedCluster == nil {
			return &selectorv1.GetSandboxResponse{
				Status:       selectorv1.GetSandboxStatus_GET_SANDBOX_STATUS_FAILED,
				ErrorMessage: "failed to select a cluster with weighted random selection",
				SandboxId:    sandboxID,
			}, nil
		}
	}

	// Create namespace based on cluster ID
	namespace := ""

	// Schedule the sandbox
	scheduleResp, err := s.schedulerClient.ScheduleSandbox(
		ctx,
		selectedCluster.Endpoint,
		sandboxID,
		namespace,
		req.Configuration,
	)
	if err != nil {
		return &selectorv1.GetSandboxResponse{
			Status:          selectorv1.GetSandboxStatus_GET_SANDBOX_STATUS_FAILED,
			ErrorMessage:    fmt.Sprintf("failed to schedule sandbox: %v", err),
			SandboxId:       sandboxID,
			ClusterId:       selectedCluster.ClusterID,
			ClusterEndpoint: selectedCluster.Endpoint,
		}, nil
	}

	// Check the scheduling status
	if scheduleResp.Status != schedulerv1.ScheduleStatus_SCHEDULE_STATUS_SCHEDULED {
		return &selectorv1.GetSandboxResponse{
			Status:          selectorv1.GetSandboxStatus_GET_SANDBOX_STATUS_FAILED,
			ErrorMessage:    scheduleResp.ErrorMessage,
			SandboxId:       sandboxID,
			ClusterId:       selectedCluster.ClusterID,
			ClusterEndpoint: selectedCluster.Endpoint,
		}, nil
	}

	// Return successful response with all details
	return &selectorv1.GetSandboxResponse{
		Status:          selectorv1.GetSandboxStatus_GET_SANDBOX_STATUS_SUCCEEDED,
		SandboxId:       sandboxID,
		ResourceName:    scheduleResp.ResourceName,
		ClusterId:       selectedCluster.ClusterID,
		ClusterEndpoint: selectedCluster.Endpoint,
	}, nil
}

// Helper structs and functions

type priorityGroup struct {
	priority int32
	clusters []*models.Cluster
}

// groupClustersByPriority groups clusters by priority and sorts in descending order
func groupClustersByPriority(clusters []*models.Cluster) []priorityGroup {
	// Create a map to group clusters by priority
	priorityMap := make(map[int32][]*models.Cluster)

	for _, cluster := range clusters {
		priorityMap[cluster.Priority] = append(priorityMap[cluster.Priority], cluster)
	}

	// Convert map to slice for sorting
	var priorityGroups []priorityGroup
	for priority, clusterGroup := range priorityMap {
		priorityGroups = append(priorityGroups, priorityGroup{
			priority: priority,
			clusters: clusterGroup,
		})
	}

	// Sort by priority in descending order (highest first)
	sort.Slice(priorityGroups, func(i, j int) bool {
		return priorityGroups[i].priority > priorityGroups[j].priority
	})

	return priorityGroups
}

// weightedRandomSelection performs weighted random selection on clusters
func weightedRandomSelection(clusters []*models.Cluster) *models.Cluster {
	// Calculate total weight
	totalWeight := int32(0)
	for _, cluster := range clusters {
		totalWeight += cluster.Weight
	}

	if totalWeight <= 0 {
		return nil
	}

	// Select a random number between 0 and total weight
	randomWeight := rand.Int31n(totalWeight)

	// Find the cluster that corresponds to the random weight
	currentWeight := int32(0)
	for _, cluster := range clusters {
		currentWeight += cluster.Weight
		if randomWeight < currentWeight {
			return cluster
		}
	}

	// This should not happen if weights are positive
	return clusters[0]
}
