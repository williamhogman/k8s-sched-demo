# Kubernetes Scheduler Demo System

A demonstration of a Kubernetes sandbox scheduling system with idempotence, expiration management, and event broadcasting.

## Overview

This demo showcases a multi-component system for scheduling and managing ephemeral Kubernetes sandbox environments:

1. **Scheduler Service** - Creates and manages sandboxes in Kubernetes clusters with TTL expiration
2. **Global Scheduler** - Frontend service that selects clusters and routes requests 
3. **Events Service** - Receives and processes sandbox termination events
4. **Demo Client** - Interacts with the system to demonstrate features

## Architecture

The system demonstrates several important architectural patterns:

- **Idempotence** - Safely handle duplicate requests using Redis
- **Event Broadcasting** - Pub/sub pattern for sandbox termination events
- **TTL Management** - Auto-expiration and cleanup of resources
- **Global Scheduling** - Cluster selection based on priority and weight

## Components

### Scheduler Service

- Creates sandbox pods in Kubernetes
- Manages idempotence with Redis or in-memory store
- Handles sandbox retention and release
- Tracks expirations and auto-cleans expired sandboxes
- Broadcasts termination events when sandboxes become unusable

### Global Scheduler

- Provides a cluster selection algorithm
- Routes sandbox requests to appropriate scheduler instances
- Handles client requests with a simple API

### Events Service 

- Receives sandbox termination events
- Processes and logs termination events
- Demonstrates event-driven architecture
- Can be extended to trigger workflows when sandboxes are terminated

### Demo Client

- Makes API calls to demonstrate system features
- Shows idempotent sandbox creation
- Demonstrates sandbox retention and release

## Running the Demo

The demo can be run with various options:

```bash
./demo.sh [options]
```

### Basic Options

- `--no-build` - Skip building binaries
- `--verbose` - Enable verbose logging
- `--release` - Demonstrate releasing the sandbox
- `--retain` - Demonstrate sandbox retention
- `--enable-events` - Enable event broadcasting

### Advanced Options

- `--scheduler-port=PORT` - Set scheduler port (default: 50052)
- `--global-port=PORT` - Set global scheduler port (default: 50051)
- `--events-port=PORT` - Set events service port (default: 50053)
- `--sandbox-ttl=DURATION` - Set sandbox TTL (default: 15m)
- `--idempotence-ttl=DURATION` - Set idempotence key TTL (default: 24h)
- `--no-redis` - Use in-memory store instead of Redis

### Example Usage

Basic demo:
```bash
./demo.sh
```

Demo with events system:
```bash
./demo.sh --enable-events --verbose
```

Full demo with all features:
```bash
./demo.sh --enable-events --verbose --retain --release
```

## Event System

The event system uses a simplified approach where only one type of event is broadcast:

- **TERMINATED** - Notification that a sandbox is no longer usable

A sandbox can become terminated for several reasons:
- Explicit release by a client
- TTL expiration
- Failure during creation or operation

The termination event includes details about what happened, allowing systems to take appropriate action when a sandbox is no longer available.

These events can be monitored in real-time when running with `--enable-events`.

## Implementation Details

- Written in Go with Connect RPC
- Uses Redis for persistent idempotence with configurable TTL
- Kubernetes client with mock mode for testing
- Uber FX for dependency injection
- Zap for structured logging 