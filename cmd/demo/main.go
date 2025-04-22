package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"connectrpc.com/connect"
	"github.com/fatih/color"

	selectorv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/global-scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/global-scheduler/v1/selectorv1connect"
)

var (
	// Command-line flags
	globalPort      = flag.Int("global-port", 50051, "The global scheduler port")
	idempotenceKey  = flag.String("key", "", "Idempotence key (defaults to auto-generated)")
	showIdempotence = flag.Bool("show-idempotence", true, "Whether to demonstrate idempotence with a second call")
	retainSandbox   = flag.Bool("retain", false, "Whether to retain the sandbox")
	retainCount     = flag.Int("retain-count", 3, "Number of times to retain the sandbox")
	retainInterval  = flag.Int("retain-interval", 5, "Seconds between retain operations")
	releaseSandbox  = flag.Bool("release", false, "Whether to release the sandbox")
	rawOutput       = flag.Bool("raw", false, "Output raw JSON responses")
	verbose         = flag.Bool("verbose", false, "Enable verbose logging")
	namespace       = flag.String("namespace", "sandbox", "Kubernetes namespace to use for sandboxes")
)

// Client wrapper for the selector service
type SelectorClient struct {
	client selectorv1connect.ClusterSelectorClient
}

// Color formatters
var (
	infoColor    = color.New(color.FgBlue).SprintFunc()
	successColor = color.New(color.FgGreen).SprintFunc()
	errorColor   = color.New(color.FgRed).SprintFunc()
	warningColor = color.New(color.FgYellow).SprintFunc()
)

// Initialize the selector client
func NewSelectorClient(port int) *SelectorClient {
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	client := selectorv1connect.NewClusterSelectorClient(
		http.DefaultClient,
		baseURL,
	)
	return &SelectorClient{client: client}
}

// Log functions - simplified
func logInfo(format string, args ...interface{}) {
	log.Printf("%s %s", infoColor("[INFO]"), fmt.Sprintf(format, args...))
}

func logSuccess(format string, args ...interface{}) {
	log.Printf("%s %s", successColor("[SUCCESS]"), fmt.Sprintf(format, args...))
}

func logError(format string, args ...interface{}) {
	log.Printf("%s %s", errorColor("[ERROR]"), fmt.Sprintf(format, args...))
}

func logWarning(format string, args ...interface{}) {
	log.Printf("%s %s", warningColor("[WARNING]"), fmt.Sprintf(format, args...))
}

// Helper to convert time to human-readable format
func formatTime(unixTime int64) string {
	t := time.Unix(unixTime, 0)
	return t.Format(time.RFC3339)
}

// Pretty print JSON responses
func prettyPrintJSON(data interface{}) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		logError("Failed to marshal JSON: %v", err)
		return
	}
	fmt.Println(string(jsonBytes))
}

// Get a sandbox from the cluster selector
func (c *SelectorClient) GetSandbox(ctx context.Context, idempotenceKey string, metadata map[string]string) (*selectorv1.GetSandboxResponse, error) {
	if *verbose {
		logInfo("Making GetSandbox request with key: %s", idempotenceKey)
	}

	req := connect.NewRequest(&selectorv1.GetSandboxRequest{
		IdempotenceKey: idempotenceKey,
		Metadata:       metadata,
	})

	resp, err := c.client.GetSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if *rawOutput {
		prettyPrintJSON(resp.Msg)
	}

	return resp.Msg, nil
}

// Retain a sandbox to extend its expiration
func (c *SelectorClient) RetainSandbox(ctx context.Context, sandboxID, clusterID string) (*selectorv1.RetainSandboxResponse, error) {
	if *verbose {
		logInfo("Making RetainSandbox request for sandbox: %s, cluster: %s", sandboxID, clusterID)
	}

	req := connect.NewRequest(&selectorv1.RetainSandboxRequest{
		SandboxId: sandboxID,
		ClusterId: clusterID,
	})

	resp, err := c.client.RetainSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if *rawOutput {
		prettyPrintJSON(resp.Msg)
	}

	return resp.Msg, nil
}

// Release a sandbox
func (c *SelectorClient) ReleaseSandbox(ctx context.Context, sandboxID, clusterID string) (*selectorv1.ReleaseSandboxResponse, error) {
	if *verbose {
		logInfo("Making ReleaseSandbox request for sandbox: %s, cluster: %s", sandboxID, clusterID)
	}

	req := connect.NewRequest(&selectorv1.ReleaseSandboxRequest{
		SandboxId: sandboxID,
		ClusterId: clusterID,
	})

	resp, err := c.client.ReleaseSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if *rawOutput {
		prettyPrintJSON(resp.Msg)
	}

	return resp.Msg, nil
}

func main() {
	// Parse command-line flags
	flag.Parse()

	// Set up logging
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	// Generate idempotence key if not provided
	if *idempotenceKey == "" {
		*idempotenceKey = fmt.Sprintf("demo-%d", time.Now().Unix())
	}

	// Create a context that can be cancelled on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		logInfo("Interrupt received, shutting down...")
		cancel()
		os.Exit(0)
	}()

	// Initialize the selector client
	client := NewSelectorClient(*globalPort)

	// Metadata for the sandbox request
	metadata := map[string]string{
		"purpose": "demo",
		"owner":   "user",
	}

	// Make the first GetSandbox request
	logInfo("Requesting sandbox with key: %s", *idempotenceKey)
	resp, err := client.GetSandbox(ctx, *idempotenceKey, metadata)
	if err != nil {
		logError("Failed to get sandbox: %v", err)
		os.Exit(1)
	}

	logSuccess("Sandbox created: %s on cluster: %s", resp.SandboxId, resp.ClusterId)

	// Store sandbox and cluster IDs for later use
	sandboxID := resp.SandboxId
	clusterID := resp.ClusterId

	// Demonstrate idempotence if enabled
	if *showIdempotence {
		time.Sleep(2 * time.Second)

		secondResp, err := client.GetSandbox(ctx, *idempotenceKey, metadata)
		if err != nil {
			logError("Failed to make second sandbox request: %v", err)
		} else {
			if secondResp.SandboxId == sandboxID {
				logSuccess("Idempotence verified: Same sandbox ID (%s)", secondResp.SandboxId)
			} else {
				logWarning("Idempotence issue: Different sandbox IDs (%s vs %s)",
					sandboxID, secondResp.SandboxId)
			}
		}
	}

	// Demonstrate sandbox retention if enabled
	if *retainSandbox {
		logInfo("Demonstrating sandbox retention...")

		for i := 1; i <= *retainCount; i++ {
			retainResp, err := client.RetainSandbox(ctx, sandboxID, clusterID)
			if err != nil {
				logError("Failed to retain sandbox: %v", err)
				break
			}

			if retainResp.Success {
				expirationTime := formatTime(retainResp.ExpirationTime)
				logSuccess("Sandbox retained. New expiration: %s", expirationTime)
			} else if retainResp.Error != "" {
				logError("Failed to retain sandbox: %s", retainResp.Error)
				break
			}

			// Wait between retain operations if not the last one
			if i < *retainCount {
				time.Sleep(time.Duration(*retainInterval) * time.Second)
			}
		}
	}

	// Demonstrate sandbox release if enabled
	if *releaseSandbox {
		time.Sleep(3 * time.Second)

		logInfo("Releasing sandbox...")
		releaseResp, err := client.ReleaseSandbox(ctx, sandboxID, clusterID)
		if err != nil {
			logError("Failed to release sandbox: %v", err)
		} else if releaseResp.Success {
			logSuccess("Sandbox released: %s", sandboxID)
		} else {
			logError("Failed to release sandbox: %s", releaseResp.Error)
		}
	}

	logSuccess("Demo completed")
}
