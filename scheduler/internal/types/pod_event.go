package types

// PodEventType represents the type of pod event
type PodEventType string

const (
	// PodAlreadyDeleted indicates the pod has already been deleted
	PodAlreadyDeleted PodEventType = "AlreadyDeleted"
	// PodToBeDeleted indicates the pod should be deleted
	PodToBeDeleted PodEventType = "ToBeDeleted"
)

// PodEvent represents a simplified Kubernetes pod event
type PodEvent struct {
	// PodName is the name of the pod
	PodName string
	// EventType indicates if the pod is already deleted or needs to be deleted
	EventType PodEventType
}

func (e PodEvent) AlreadyDeleted() bool {
	return e.EventType == PodAlreadyDeleted
}

func (e PodEvent) ToBeDeleted() bool {
	return e.EventType == PodToBeDeleted
}
