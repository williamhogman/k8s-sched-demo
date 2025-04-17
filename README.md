# Kubernetes Scheduler Demo

This project demonstrates a two-tier scheduling system for Kubernetes workloads:

1. **Global Scheduler Service** (formerly "Cluster Selector"): Decides which Kubernetes cluster should run a workload based on priority and weighted random selection.
2. **Scheduler Service**: Schedules the workload on the selected Kubernetes cluster using the K8s API.

> **Note:** The "Cluster Selector" service has been renamed to "Global Scheduler" to better reflect its role as the higher-level scheduler in the system. The code repository still contains references to "cluster-selector" that will be refactored in future updates.

## Architecture

- **Global Scheduler** maintains a list of available clusters with priority and weight information.
- When a scheduling request is made, it selects the highest priority cluster(s).
- If multiple clusters have the same priority, it uses weighted random selection based on the weight of each cluster.
- After selecting a cluster, it connects to the **Scheduler Service** on that cluster via gRPC.
- The **Scheduler Service** takes the request and schedules a Kubernetes pod for the sandbox.

## Simplified API

The system includes a simplified API through the Global Scheduler's `GetSandbox` endpoint:

- **Single Call Operation**: Clients use a single call to the Global Scheduler for both cluster selection and sandbox scheduling.
- **End-to-End Process**: The Global Scheduler handles both cluster selection and sandbox scheduling internally.
- **Streamlined Client Experience**: Clients only need to specify configuration (like image and command) to receive a fully scheduled sandbox.
- **Internal Resource Management**: CPU and memory resources are managed internally by the scheduler with default settings, keeping resource allocation decisions encapsulated within the system.

This simplification reduces client complexity and provides a more cohesive scheduling experience.

## Project Structure

```
.
├── cluster-selector/           # Global Scheduler service (directory to be renamed)
│   ├── cmd/server/             # gRPC server implementation
│   ├── internal/               # Internal packages
│   │   ├── client/             # Client for connecting to scheduler
│   │   ├── service/            # Implementation of selection logic
│   │   └── storage/            # Storage for cluster information
│   └── proto/                  # Protocol buffer definitions
├── scheduler/                  # Kubernetes scheduler service
│   ├── cmd/server/             # gRPC server implementation 
│   ├── internal/               # Internal packages
│   │   ├── k8sclient/          # Kubernetes client wrapper
│   │   └── service/            # Implementation of scheduling logic
│   └── proto/                  # Protocol buffer definitions
└── cmd/client/                 # Example client implementation
```

## Building and Running

### Prerequisites

- Go 1.21 or later
- Protocol Buffers compiler (`protoc`)
- A Kubernetes cluster (for running the scheduler service)

### Building the Project

```bash
make build
```

### Running the Services

1. Start the Scheduler service (using the mock client for testing):

```bash
./bin/scheduler --mock=true --port=50052
```

2. Start the Global Scheduler service:

```bash
./bin/global-scheduler
```

3. Run the client to get a sandbox with the simplified API:

```bash
./bin/client --image="nginx:latest" --command="nginx -g 'daemon off;'"
```

Or run the demo script to see everything in action:

```bash
./demo.sh
```

## Implementation Details

### Cluster Selection Algorithm

1. Filter clusters by the highest available priority
2. If multiple clusters have the same priority, use weighted random selection:
   - Each cluster's weight determines how many "slots" it gets in the selection pool
   - Randomly select a slot, which corresponds to a cluster

### Kubernetes Scheduling

The scheduler service creates a Kubernetes pod with:
- Labels identifying the sandbox
- Default resource allocations (CPU and memory)
- Configuration (image, command) passed in the request 