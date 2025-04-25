# Kubernetes Scheduler Demo

This project demonstrates a custom Kubernetes-based scheduler system for managing sandboxes in a Kubernetes cluster. The system provides project-based sandbox allocation, routing, and lifecycle management.

## Components

- **Scheduler Service**: Manages sandbox allocation, scheduling, and lifecycle
- **Mock Sandbox**: A simple sandbox implementation for demonstration
- **Demo Client**: A client application that demonstrates the system

## Prerequisites

- Docker Desktop with Kubernetes enabled or a Kubernetes cluster
- Go 1.23 or later
- kubectl configured to access your cluster
- Buf for Protocol Buffer generation

## Quick Start

The easiest way to run the demo is using the demo script:

```bash
# Run the full demo (build, deploy to K8s, and run client)
./demo.sh

# Just show the client demonstration (assumes services are already running)
./demo.sh --client-only
```

## Manual Deployment

### 1. Build Docker Images

```bash
# Build the scheduler and mock-sandbox Docker images
make docker-build
```

### 2. Deploy to Kubernetes

```bash
# Deploy to Kubernetes
make k8s-deploy
```

### 3. Run the Demo Client

```bash
# Build and run the demo client
make build
./bin/demo --scheduler-host=scheduler.sandbox.svc.cluster.local
```

## Development

```bash
# Generate Protocol Buffer files
make proto

# Build all binaries
make build

# Run the scheduler locally
make run-scheduler

# Run the activator service locally
make run-activator
```

## Usage Examples

### Get a Sandbox for a Project

```bash
./bin/demo --project-id=my-project
```

### Release a Sandbox

```bash
./bin/demo --project-id=my-project --release
```

### Retain a Sandbox (extend expiration)

```bash
./bin/demo --project-id=my-project --retain
```

## Kubernetes Resources

The demo creates the following Kubernetes resources:

- `sandbox` namespace for all resources
- Redis deployment for idempotence management
- Scheduler service deployment
- On-demand sandbox pods as requested by clients

## Architecture

1. Client requests a sandbox for a project
2. Scheduler service creates or reuses a sandbox
3. Scheduler creates a headless service for routing to the sandbox
4. Client can access the sandbox via the service hostname
5. Sandboxes are automatically cleaned up after TTL expiration

## License

MIT 