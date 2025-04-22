# K8s Scheduler Sandbox

A simple Go application that serves as a sandbox for the K8s Scheduler demo.

## Features

- HTTP server running on port 8000
- Health check endpoint at `/health`
- Readiness check endpoint at `/ready`
- Endpoint to crash the container at `/crash`
- Endpoint to exit gracefully at `/exit`
- Endpoint to simulate slow shutdown at `/slow-shutdown`
- Graceful handling of SIGTERM signals for proper Kubernetes pod termination

## Building

```bash
# Build the Docker image
docker build -t localhost:5001/sandbox:latest .

# Push to local registry
docker push localhost:5001/sandbox:latest
```

## Usage

The sandbox is designed to be deployed as a Kubernetes pod. It provides the following endpoints:

- `GET /`: Returns a simple message indicating the sandbox is running
- `GET /health`: Returns a 200 OK response for health checks
- `GET /ready`: Returns a 200 OK response for readiness checks
- `GET /crash`: Causes the container to crash by panicking
- `GET /exit`: Causes the container to exit gracefully with code 0
- `GET /slow-shutdown`: Simulates a slow shutdown by sleeping for 30 seconds before exiting

## Signal Handling

The sandbox application properly handles termination signals:

- `SIGTERM`: Initiates a graceful shutdown with a 10-second timeout
- `SIGINT`: Also initiates a graceful shutdown with a 10-second timeout

This ensures that when Kubernetes sends a termination signal to the pod, the application has time to clean up resources and shut down properly.

## Implementation Details

The sandbox is built using:

- Go 1.21
- FX for dependency injection
- Gorilla Mux for HTTP routing
- Zap for logging

## Integration with K8s Scheduler

The sandbox is designed to work with the K8s Scheduler demo. The scheduler will:

1. Create a pod using this sandbox image
2. Configure health and readiness probes to use the `/health` and `/ready` endpoints
3. Monitor the pod's status and handle events 