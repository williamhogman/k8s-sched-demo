package models

// Cluster represents a Kubernetes cluster that can be selected for sandbox deployment
type Cluster struct {
	// Unique identifier for the cluster
	ClusterID string
	// Priority of the cluster (higher values are preferred)
	Priority int32
	// Weight for weighted random selection
	Weight int32
	// Location information about the cluster
	Region string
	// Endpoint for connecting to the scheduler service on this cluster
	Endpoint string
}

// NewCluster creates a new cluster instance
func NewCluster(clusterID string, priority, weight int32, region, endpoint string) *Cluster {
	return &Cluster{
		ClusterID: clusterID,
		Priority:  priority,
		Weight:    weight,
		Region:    region,
		Endpoint:  endpoint,
	}
}
