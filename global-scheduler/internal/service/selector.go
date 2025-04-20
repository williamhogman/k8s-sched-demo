package service

import (
	"context"
	"fmt"
	"math/rand"
	"sort"

	selectorv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/global-scheduler/v1"
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
	// Extract idempotence key
	idempotenceKey := req.IdempotenceKey
	if idempotenceKey == "" {
		return &selectorv1.GetSandboxResponse{
			Success: false,
			Error:   "idempotence key cannot be empty",
		}, nil
	}

	// Get all available clusters
	clusters := s.clusters
	if len(clusters) == 0 {
		return &selectorv1.GetSandboxResponse{
			Success: false,
			Error:   "no clusters available",
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
			Success: false,
			Error:   "failed to group clusters by priority",
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
				Success: false,
				Error:   "failed to select a cluster with weighted random selection",
			}, nil
		}
	}

	// Create namespace based on cluster ID
	namespace := ""

	// Schedule the sandbox
	scheduleResp, err := s.schedulerClient.ScheduleSandbox(
		ctx,
		selectedCluster.Endpoint,
		idempotenceKey,
		namespace,
		req.Metadata,
	)
	if err != nil {
		return &selectorv1.GetSandboxResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to schedule sandbox: %v", err),
		}, nil
	}

	// Check if scheduling was successful
	if !scheduleResp.Success {
		return &selectorv1.GetSandboxResponse{
			Success: false,
			Error:   scheduleResp.Error,
		}, nil
	}

	// Return successful response with sandbox ID and cluster ID
	return &selectorv1.GetSandboxResponse{
		Success:   true,
		SandboxId: scheduleResp.SandboxId,
		ClusterId: selectedCluster.ClusterID,
	}, nil
}

// ReleaseSandbox handles releasing a sandbox from a specific cluster
func (s *SelectorService) ReleaseSandbox(ctx context.Context, req *selectorv1.ReleaseSandboxRequest) (*selectorv1.ReleaseSandboxResponse, error) {
	// Extract sandbox ID and cluster ID
	sandboxID := req.SandboxId
	clusterID := req.ClusterId

	if sandboxID == "" {
		return &selectorv1.ReleaseSandboxResponse{
			Success: false,
			Error:   "sandbox ID cannot be empty",
		}, nil
	}

	if clusterID == "" {
		return &selectorv1.ReleaseSandboxResponse{
			Success: false,
			Error:   "cluster ID cannot be empty",
		}, nil
	}

	// Find the cluster with the given ID
	cluster, ok := s.clusters[clusterID]
	if !ok {
		return &selectorv1.ReleaseSandboxResponse{
			Success: false,
			Error:   fmt.Sprintf("cluster with ID %s not found", clusterID),
		}, nil
	}

	// Call the scheduler service to release the sandbox
	releaseResp, err := s.schedulerClient.ReleaseSandbox(
		ctx,
		cluster.Endpoint,
		sandboxID,
	)
	if err != nil {
		return &selectorv1.ReleaseSandboxResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to release sandbox: %v", err),
		}, nil
	}

	// Return the response from the scheduler
	return &selectorv1.ReleaseSandboxResponse{
		Success: releaseResp.Success,
		Error:   releaseResp.Error,
	}, nil
}

// RetainSandbox handles extending the expiration time of a sandbox in a specific cluster
func (s *SelectorService) RetainSandbox(ctx context.Context, req *selectorv1.RetainSandboxRequest) (*selectorv1.RetainSandboxResponse, error) {
	// Extract sandbox ID and cluster ID
	sandboxID := req.SandboxId
	clusterID := req.ClusterId

	if sandboxID == "" {
		return &selectorv1.RetainSandboxResponse{
			Success: false,
			Error:   "sandbox ID cannot be empty",
		}, nil
	}

	if clusterID == "" {
		return &selectorv1.RetainSandboxResponse{
			Success: false,
			Error:   "cluster ID cannot be empty",
		}, nil
	}

	// Find the cluster with the given ID
	cluster, ok := s.clusters[clusterID]
	if !ok {
		return &selectorv1.RetainSandboxResponse{
			Success: false,
			Error:   fmt.Sprintf("cluster with ID %s not found", clusterID),
		}, nil
	}

	// Call the scheduler service to retain the sandbox
	retainResp, err := s.schedulerClient.RetainSandbox(
		ctx,
		cluster.Endpoint,
		sandboxID,
	)
	if err != nil {
		return &selectorv1.RetainSandboxResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to retain sandbox: %v", err),
		}, nil
	}

	// Return the response from the scheduler
	return &selectorv1.RetainSandboxResponse{
		Success:        retainResp.Success,
		Error:          retainResp.Error,
		ExpirationTime: retainResp.ExpirationTime,
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
