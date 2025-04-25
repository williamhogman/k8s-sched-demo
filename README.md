# Kubernetes Scheduler Demo System

A demonstration of a Kubernetes sandbox scheduling system with idempotence, expiration management, and local event handling.

## Overview

This demo showcases a multi-component system for scheduling and managing ephemeral Kubernetes sandbox environments:

1. **Scheduler Service** - Creates and manages sandboxes in Kubernetes clusters with TTL expiration
2. **Demo Client** - Interacts with the system to demonstrate features

## Architecture

The system demonstrates several important architectural patterns:

- **Idempotence** - Safely handle duplicate requests using Redis
- **Local Event Handling** - Internal event processing for sandbox lifecycle events
- **TTL Management** - Auto-expiration and cleanup of resources

## Components

### Scheduler Service

- Creates sandbox pods in Kubernetes
- Manages idempotence with Redis or in-memory store
- Handles sandbox retention and release
- Tracks expirations and auto-cleans expired sandboxes
- Processes pod deletion events to clean up project-sandbox mappings

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

### Advanced Options

- `--scheduler-port=PORT` - Set scheduler port (default: 50051)
- `--sandbox-ttl=DURATION` - Set sandbox TTL (default: 15m)
- `--idempotence-ttl=DURATION` - Set idempotence key TTL (default: 24h)
- `--no-redis` - Use in-memory store instead of Redis

### Example Usage

Basic demo:
```bash
./demo.sh
```

Full demo with all features:
```bash
./demo.sh --verbose --retain --release
```

## Implementation Details

- Written in Go with Connect RPC
- Uses Redis for persistent idempotence with configurable TTL
- Kubernetes client with mock mode for testing
- Uber FX for dependency injection
- Zap for structured logging 