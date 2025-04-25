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

	schedulerv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/scheduler/v1"
	"github.com/williamhogman/k8s-sched-demo/gen/will/scheduler/v1/schedulerv1connect"
)

var (
	// Command-line flags
	schedulerPort   = flag.Int("scheduler-port", 50051, "The scheduler port")
	projectID       = flag.String("project-id", "", "Project ID (defaults to auto-generated)")
	showIdempotence = flag.Bool("show-idempotence", true, "Whether to demonstrate idempotence with a second call")
	retainSandbox   = flag.Bool("retain", false, "Whether to retain the sandbox")
	retainCount     = flag.Int("retain-count", 3, "Number of times to retain the sandbox")
	retainInterval  = flag.Int("retain-interval", 5, "Seconds between retain operations")
	releaseSandbox  = flag.Bool("release", false, "Whether to release the sandbox")
	rawOutput       = flag.Bool("raw", false, "Output raw JSON responses")
	verbose         = flag.Bool("verbose", false, "Enable verbose logging")
	namespace       = flag.String("namespace", "sandbox", "Kubernetes namespace to use for sandboxes")
	schedulerHost   = flag.String("scheduler-host", "localhost", "The scheduler host")
)

// Client wrapper for the project service
type ProjectClient struct {
	client schedulerv1connect.ProjectServiceClient
}

// Color formatters
var (
	infoColor    = color.New(color.FgBlue).SprintFunc()
	successColor = color.New(color.FgGreen).SprintFunc()
	errorColor   = color.New(color.FgRed).SprintFunc()
	warningColor = color.New(color.FgYellow).SprintFunc()
)

// Initialize the project client
func NewProjectClient(port int, hostname string) *ProjectClient {
	baseURL := fmt.Sprintf("http://%s:%d", hostname, port)
	client := schedulerv1connect.NewProjectServiceClient(
		http.DefaultClient,
		baseURL,
	)
	return &ProjectClient{client: client}
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

// Get a sandbox for a project
func (c *ProjectClient) GetProjectSandbox(ctx context.Context, projectID string, metadata map[string]string) (*schedulerv1.GetProjectSandboxResponse, error) {
	if *verbose {
		logInfo("Making GetProjectSandbox request for project: %s", projectID)
	}

	req := connect.NewRequest(&schedulerv1.GetProjectSandboxRequest{
		ProjectId:       projectID,
		Metadata:        metadata,
		WaitForCreation: true,
	})

	resp, err := c.client.GetProjectSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if *rawOutput {
		prettyPrintJSON(resp.Msg)
	}

	return resp.Msg, nil
}

// Get the status of a project's sandbox
func (c *ProjectClient) GetProjectStatus(ctx context.Context, projectID string) (*schedulerv1.GetProjectStatusResponse, error) {
	if *verbose {
		logInfo("Making GetProjectStatus request for project: %s", projectID)
	}

	req := connect.NewRequest(&schedulerv1.GetProjectStatusRequest{
		ProjectId: projectID,
	})

	resp, err := c.client.GetProjectStatus(ctx, req)
	if err != nil {
		return nil, err
	}

	if *rawOutput {
		prettyPrintJSON(resp.Msg)
	}

	return resp.Msg, nil
}

// Release a project's sandbox
func (c *ProjectClient) ReleaseProjectSandbox(ctx context.Context, projectID string) (*schedulerv1.ReleaseProjectSandboxResponse, error) {
	if *verbose {
		logInfo("Making ReleaseProjectSandbox request for project: %s", projectID)
	}

	req := connect.NewRequest(&schedulerv1.ReleaseProjectSandboxRequest{
		ProjectId: projectID,
	})

	resp, err := c.client.ReleaseProjectSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if *rawOutput {
		prettyPrintJSON(resp.Msg)
	}

	return resp.Msg, nil
}

// Helper to convert status to string
func statusToString(status schedulerv1.ProjectSandboxStatus) string {
	switch status {
	case schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ACTIVE:
		return "ACTIVE"
	case schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_CREATING:
		return "CREATING"
	case schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_NOT_FOUND:
		return "NOT_FOUND"
	case schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ERROR:
		return "ERROR"
	default:
		return "UNSPECIFIED"
	}
}

func main() {
	// Parse command-line flags
	flag.Parse()

	// Set up logging
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	// Generate project ID if not provided
	if *projectID == "" {
		*projectID = fmt.Sprintf("demo-project-%d", time.Now().Unix())
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

	// Initialize the project client
	client := NewProjectClient(*schedulerPort, *schedulerHost)

	// Metadata for the sandbox request
	metadata := map[string]string{
		"purpose": "demo",
		"owner":   "user",
	}

	// Get a sandbox for the project
	logInfo("Requesting sandbox for project: %s", *projectID)
	resp, err := client.GetProjectSandbox(ctx, *projectID, metadata)
	if err != nil {
		logError("Failed to get project sandbox: %v", err)
		os.Exit(1)
	}

	logSuccess("Project sandbox created: %s (Status: %s, Hostname: %s)", resp.SandboxId, statusToString(resp.Status), resp.Hostname)

	// Store sandbox ID for later use
	sandboxID := resp.SandboxId

	// Demonstrate idempotence if enabled
	if *showIdempotence {
		time.Sleep(2 * time.Second)

		secondResp, err := client.GetProjectSandbox(ctx, *projectID, metadata)
		if err != nil {
			logError("Failed to make second project sandbox request: %v", err)
		} else {
			if secondResp.SandboxId == sandboxID {
				logSuccess("Idempotence verified: Same sandbox ID (%s)", secondResp.SandboxId)
			} else {
				logWarning("Idempotence issue: Different sandbox IDs (%s vs %s)",
					sandboxID, secondResp.SandboxId)
			}
		}
	}

	// Check project status
	statusResp, err := client.GetProjectStatus(ctx, *projectID)
	if err != nil {
		logError("Failed to get project status: %v", err)
	} else {
		logInfo("Project status: %s", statusToString(statusResp.Status))
	}

	// Demonstrate sandbox retention if enabled
	if *retainSandbox {
		logInfo("Demonstrating sandbox retention...")

		for i := 1; i <= *retainCount; i++ {
			// Get project status to check sandbox
			statusResp, err := client.GetProjectStatus(ctx, *projectID)
			if err != nil {
				logError("Failed to get project status: %v", err)
				break
			}

			if statusResp.Status != schedulerv1.ProjectSandboxStatus_PROJECT_SANDBOX_STATUS_ACTIVE {
				logError("Sandbox is not active (status: %s)", statusToString(statusResp.Status))
				break
			}

			logSuccess("Sandbox retained. Status: %s", statusToString(statusResp.Status))

			// Wait between retain operations if not the last one
			if i < *retainCount {
				time.Sleep(time.Duration(*retainInterval) * time.Second)
			}
		}
	}

	// Demonstrate sandbox release if enabled
	if *releaseSandbox {
		time.Sleep(3 * time.Second)

		logInfo("Releasing project sandbox...")
		releaseResp, err := client.ReleaseProjectSandbox(ctx, *projectID)
		if err != nil {
			logError("Failed to release project sandbox: %v", err)
		} else if releaseResp.Success {
			logSuccess("Project sandbox released: %s", releaseResp.ReleasedSandboxId)
		} else {
			logError("Failed to release project sandbox")
		}

		// Verify the sandbox was released
		statusResp, err := client.GetProjectStatus(ctx, *projectID)
		if err != nil {
			logError("Failed to get final project status: %v", err)
		} else {
			logInfo("Final project status: %s", statusToString(statusResp.Status))
		}
	}

	logSuccess("Demo completed")
}
