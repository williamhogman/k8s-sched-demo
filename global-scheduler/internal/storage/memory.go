package storage

import (
	"errors"
	"sync"

	"github.com/williamhogman/k8s-sched-demo/global-scheduler/internal/models"
)

var (
	ErrClusterNotFound = errors.New("cluster not found")
	ErrClusterExists   = errors.New("cluster already exists")
)

// MemoryStorage implements an in-memory storage for cluster information
type MemoryStorage struct {
	mu       sync.RWMutex
	clusters map[string]*models.Cluster
}

// NewMemoryStorage creates a new memory storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		clusters: make(map[string]*models.Cluster),
	}
}

// AddCluster adds a new cluster to storage
func (m *MemoryStorage) AddCluster(cluster *models.Cluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clusters[cluster.ClusterID]; exists {
		return ErrClusterExists
	}

	// Make a copy to avoid shared references
	clusterCopy := &models.Cluster{
		ClusterID: cluster.ClusterID,
		Priority:  cluster.Priority,
		Weight:    cluster.Weight,
		Region:    cluster.Region,
		Endpoint:  cluster.Endpoint,
	}

	m.clusters[cluster.ClusterID] = clusterCopy
	return nil
}

// UpdateCluster updates an existing cluster
func (m *MemoryStorage) UpdateCluster(cluster *models.Cluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clusters[cluster.ClusterID]; !exists {
		return ErrClusterNotFound
	}

	// Make a copy to avoid shared references
	clusterCopy := &models.Cluster{
		ClusterID: cluster.ClusterID,
		Priority:  cluster.Priority,
		Weight:    cluster.Weight,
		Region:    cluster.Region,
		Endpoint:  cluster.Endpoint,
	}

	m.clusters[cluster.ClusterID] = clusterCopy
	return nil
}

// GetCluster retrieves a cluster by ID
func (m *MemoryStorage) GetCluster(clusterID string) (*models.Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cluster, exists := m.clusters[clusterID]
	if !exists {
		return nil, ErrClusterNotFound
	}

	// Return a copy to avoid shared references
	return &models.Cluster{
		ClusterID: cluster.ClusterID,
		Priority:  cluster.Priority,
		Weight:    cluster.Weight,
		Region:    cluster.Region,
		Endpoint:  cluster.Endpoint,
	}, nil
}

// ListClusters returns all clusters
func (m *MemoryStorage) ListClusters() []*models.Cluster {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*models.Cluster, 0, len(m.clusters))
	for _, cluster := range m.clusters {
		result = append(result, &models.Cluster{
			ClusterID: cluster.ClusterID,
			Priority:  cluster.Priority,
			Weight:    cluster.Weight,
			Region:    cluster.Region,
			Endpoint:  cluster.Endpoint,
		})
	}

	return result
}

// ListClustersByRegion returns clusters filtered by region
func (m *MemoryStorage) ListClustersByRegion(region string) []*models.Cluster {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*models.Cluster, 0)
	for _, cluster := range m.clusters {
		if cluster.Region == region {
			result = append(result, &models.Cluster{
				ClusterID: cluster.ClusterID,
				Priority:  cluster.Priority,
				Weight:    cluster.Weight,
				Region:    cluster.Region,
				Endpoint:  cluster.Endpoint,
			})
		}
	}

	return result
}

// DeleteCluster removes a cluster by ID
func (m *MemoryStorage) DeleteCluster(clusterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clusters[clusterID]; !exists {
		return ErrClusterNotFound
	}

	delete(m.clusters, clusterID)
	return nil
}
