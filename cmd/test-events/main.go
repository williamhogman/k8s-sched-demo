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

	// Send to channel for processing
	select {
	case s.eventChan <- event:
		// Event sent to channel
	default:
		// Channel full or closed, just log
		log.Printf("Event channel full or closed")
	}

	// Return success
	return connect.NewResponse(&schedulerv1.SandboxEventResponse{
		Success: true,
	}), nil
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
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Start processing events in a goroutine
	go processEvents(ctx, eventChan)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	log.Println("Shutting down...")

	// Create a shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Gracefully shutdown the server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Shutdown failed: %v", err)
	}
}

// processEvents handles events from the channel
func processEvents(ctx context.Context, eventChan <-chan *schedulerv1.SandboxEvent) {
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				// Channel closed
				return
			}

			if event.EventType == schedulerv1.SandboxEventType_SANDBOX_EVENT_TYPE_TERMINATED {
				// Process the termination event
				log.Printf("Sandbox terminated: %s at %s",
					event.SandboxId,
					time.Unix(event.Timestamp, 0).Format(time.RFC3339))
			}

		case <-ctx.Done():
			// Context canceled
			return
		}
	}
}
