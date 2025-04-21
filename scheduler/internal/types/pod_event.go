package types

import "time"

// PodEventType represents the type of pod event
type PodEventType string

const (
	// PodEventFailed indicates that the pod has failed
	PodEventFailed PodEventType = "Failed"
	// PodEventSucceeded indicates that the pod completed successfully
	PodEventSucceeded PodEventType = "Succeeded"
	// PodEventTerminated indicates that the pod was terminated
	PodEventTerminated PodEventType = "Terminated"
	// PodEventUnschedulable indicates that the pod cannot be scheduled
	PodEventUnschedulable PodEventType = "Unschedulable"
)

// PodEvent represents a Kubernetes pod event that needs to be propagated
type PodEvent struct {
	// PodName is the name of the pod
	PodName string
	// EventType is the type of the event
	EventType PodEventType
	// Reason provides additional context about the event
	Reason string
	// Message provides a human-readable explanation
	Message string
	// Timestamp is when the event occurred
	Timestamp time.Time
}
