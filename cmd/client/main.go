package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"connectrpc.com/connect"
	selectorv1 "github.com/williamhogman/k8s-sched-demo/gen/go/will/global-scheduler/v1"
	selectorv1connect "github.com/williamhogman/k8s-sched-demo/gen/will/global-scheduler/v1/selectorv1connect"
)

var (
	selectorAddr = flag.String("selector", "http://localhost:50051", "The selector server address")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create HTTP client
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	conn := selectorv1connect.NewClusterSelectorClient(client, *selectorAddr)

	resp, err := conn.GetSandbox(ctx, connect.NewRequest(&selectorv1.GetSandboxRequest{
		SandboxId:     "123",
		Configuration: map[string]string{},
	}))
	if err != nil {
		log.Fatalf("Failed to get sandbox: %v", err)
	}

	log.Printf("Sandbox successfully created:")
	log.Printf("  Sandbox ID: %s", resp.Msg.SandboxId)
	log.Printf("  Resource Name: %s", resp.Msg.ResourceName)
	log.Printf("  Cluster ID: %s", resp.Msg.ClusterId)
	log.Printf("  Cluster Endpoint: %s", resp.Msg.ClusterEndpoint)
}
