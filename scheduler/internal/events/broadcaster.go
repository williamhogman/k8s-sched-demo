package events

import (
	"context"
	"fmt"
	"time"

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"go.uber.org/zap"
)

// Event represents a sandbox event to be broadcast
type Event struct {
	SandboxID string
	Type      schedulerv1.SandboxEventType
}

// BroadcasterInterface defines the interface for the event broadcaster
type BroadcasterInterface interface {
	// BroadcastEvent sends a sandbox event to the configured endpoint
	BroadcastEvent(ctx context.Context, event Event) error
	// Close closes the event broadcaster
	Close() error
}

// Broadcaster implements the event broadcaster service
type Broadcaster struct {
	config     config.EventBroadcasterConfig
	logger     *zap.Logger
	eventChan  chan Event
	ctx        context.Context
	cancelFunc context.CancelFunc
	client     *SandboxEventClient
}

// NewBroadcaster creates a new event broadcaster
func NewBroadcaster(cfg *config.Config, logger *zap.Logger) *Broadcaster {
	ctx, cancel := context.WithCancel(context.Background())

	b := &Broadcaster{
		config:     cfg.EventBroadcaster,
		logger:     logger.Named("event-broadcaster"),
		eventChan:  make(chan Event, cfg.EventBroadcaster.BufferSize),
		ctx:        ctx,
		cancelFunc: cancel,
	}

	// Create client if broadcasting is enabled
	if cfg.EventBroadcaster.Enabled {
		b.client = NewSandboxEventClient(cfg.EventBroadcaster.Endpoint)
		// Start the event processing goroutine
		go b.processEvents()
	} else {
		b.logger.Info("Event broadcasting is disabled")
	}

	return b
}

// BroadcastEvent adds an event to the broadcast queue
func (b *Broadcaster) BroadcastEvent(ctx context.Context, event Event) error {
	if !b.config.Enabled {
		// If broadcasting is disabled, just log and return success
		b.logger.Debug("Event broadcasting disabled, ignoring event",
			zap.String("sandboxID", event.SandboxID),
			zap.Int32("type", int32(event.Type)))
		return nil
	}

	// Try to add to channel with context deadline
	select {
	case b.eventChan <- event:
		b.logger.Debug("Added event to broadcast queue",
			zap.String("sandboxID", event.SandboxID),
			zap.Int32("type", int32(event.Type)))
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context deadline exceeded while enqueueing event")
	default:
		// Channel is full
		b.logger.Warn("Event channel is full, dropping event",
			zap.String("sandboxID", event.SandboxID),
			zap.Int32("type", int32(event.Type)))
		return fmt.Errorf("event channel is full, event dropped")
	}
}

// processEvents handles sending events from the channel to the event service
func (b *Broadcaster) processEvents() {
	b.logger.Info("Starting event broadcaster",
		zap.String("endpoint", b.config.Endpoint))

	for {
		select {
		case event, ok := <-b.eventChan:
			if !ok {
				// Channel closed, exit
				b.logger.Info("Event channel closed, stopping broadcaster")
				return
			}

			// Process the event
			err := b.sendEventWithRetry(event)
			if err != nil {
				b.logger.Error("Failed to send event after retries",
					zap.String("sandboxID", event.SandboxID),
					zap.Int32("type", int32(event.Type)),
					zap.Error(err))
			}

		case <-b.ctx.Done():
			// Context canceled, exit
			b.logger.Info("Event broadcaster context canceled, stopping")
			return
		}
	}
}

// sendEventWithRetry sends an event to the event service with retries
func (b *Broadcaster) sendEventWithRetry(event Event) error {
	var lastErr error

	// Create a new context for just this sending operation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to send the event with retries
	for i := 0; i <= b.config.MaxRetries; i++ {
		if i > 0 {
			// Wait before retrying
			time.Sleep(b.config.RetryDelay)
			b.logger.Info("Retrying event send",
				zap.String("sandboxID", event.SandboxID),
				zap.Int32("type", int32(event.Type)),
				zap.Int("attempt", i+1),
				zap.Int("maxRetries", b.config.MaxRetries))
		}

		err := b.sendEvent(ctx, event)
		if err == nil {
			// Success
			return nil
		}

		lastErr = err
		b.logger.Warn("Failed to send event",
			zap.String("sandboxID", event.SandboxID),
			zap.Int32("type", int32(event.Type)),
			zap.Int("attempt", i+1),
			zap.Error(err))
	}

	return fmt.Errorf("failed after %d attempts: %v", b.config.MaxRetries+1, lastErr)
}

// sendEvent sends a single event to the event service
func (b *Broadcaster) sendEvent(ctx context.Context, event Event) error {
	// Convert the event to the protobuf format
	protoEvent := &schedulerv1.SandboxEvent{
		SandboxId: event.SandboxID,
		EventType: event.Type,
		Timestamp: time.Now().Unix(),
	}

	// Use the client to send the event
	_, err := b.client.SendEvent(ctx, protoEvent)
	if err != nil {
		return fmt.Errorf("failed to send event: %v", err)
	}

	b.logger.Info("Successfully sent event",
		zap.String("sandboxID", event.SandboxID),
		zap.Int32("type", int32(event.Type)),
		zap.String("endpoint", b.config.Endpoint))

	return nil
}

// Close closes the event broadcaster
func (b *Broadcaster) Close() error {
	b.cancelFunc()
	// Wait for all events to be processed
	close(b.eventChan)
	return nil
}
