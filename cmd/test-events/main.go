package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"connectrpc.com/connect"
	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
)

var (
	port = flag.Int("port", 50053, "The server port")
)

// EventService implements the sandbox event service handler
type EventService struct {
	eventChan chan *schedulerv1.SandboxEvent
}

// SendSandboxEvent handles incoming sandbox events
func (s *EventService) SendSandboxEvent(
	ctx context.Context,
	req *connect.Request[schedulerv1.SandboxEvent],
) (*connect.Response[schedulerv1.SandboxEventResponse], error) {
	event := req.Msg

	// Log the event
	log.Printf("📩 Received event: sandboxID=%s, type=%s",
		event.SandboxId,
		getEventTypeString(event.EventType))

	// Send to channel for processing
	select {
	case s.eventChan <- event:
		// Event sent to channel
	default:
		// Channel full or closed, just log
		log.Printf("⚠️ Event channel full or closed, event not processed")
	}

	// Return success
	return connect.NewResponse(&schedulerv1.SandboxEventResponse{
		Success: true,
	}), nil
}

// getEventTypeString returns a human-readable string for the event type
func getEventTypeString(eventType schedulerv1.SandboxEventType) string {
	switch eventType {
	case schedulerv1.SandboxEventType_SANDBOX_EVENT_TYPE_TERMINATED:
		return "TERMINATED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", eventType)
	}
}

func main() {
	flag.Parse()

	// Create the event service
	eventChan := make(chan *schedulerv1.SandboxEvent, 100)
	eventService := &EventService{
		eventChan: eventChan,
	}

	// Set up the HTTP routes with Connect handlers
	mux := http.NewServeMux()
	path, handler := schedulerv1connect.NewSandboxEventServiceHandler(eventService)
	mux.Handle(path, handler)

	// Create a context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create and start the HTTP server
	addr := fmt.Sprintf(":%d", *port)
	server := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	// Start the server in a goroutine
	go func() {
		log.Printf("Starting event receiver on %s\n", addr)
		log.Printf("Connect API path: %s\n", path)
		log.Printf("This service will receive notifications when sandboxes are terminated")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Start processing events in a goroutine
	go processEvents(ctx, eventChan)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	log.Println("Shutting down server...")

	// Create a shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Gracefully shutdown the server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server gracefully stopped")
}

// processEvents handles events from the channel
func processEvents(ctx context.Context, eventChan <-chan *schedulerv1.SandboxEvent) {
	log.Println("Event processor started")
	log.Println("Listening for sandbox termination events (when sandboxes are no longer usable)")

	// In a real implementation, this would store events, forward them, etc.
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				// Channel closed
				log.Println("Event channel closed, stopping processor")
				return
			}

			if event.EventType == schedulerv1.SandboxEventType_SANDBOX_EVENT_TYPE_TERMINATED {
				// Process the termination event
				log.Printf("🚨 SANDBOX TERMINATED: %s", event.SandboxId)
				log.Printf("   Time: %s", time.Unix(event.Timestamp, 0).Format(time.RFC3339))
			}

		case <-ctx.Done():
			// Context canceled
			log.Println("Context canceled, stopping event processor")
			return
		}
	}
}
