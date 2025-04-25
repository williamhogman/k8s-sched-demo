#!/bin/bash
set -euo pipefail

# ================================================================
# K8s Scheduler Demo
# 
# This script demonstrates the K8s scheduler demo system
# ================================================================

# Color codes for better readability
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default configuration
BUILD=true
SCHEDULER_PORT=50051
DEMO_PORT=50053
HEALTH_CHECK_TIMEOUT=10
RELEASE_SANDBOX=false
RETAIN_SANDBOX=false
RETAIN_COUNT=3
RETAIN_INTERVAL=5
USE_REDIS=true
REDIS_PORT=6379
REDIS_ADDR="localhost:$REDIS_PORT"
REDIS_PASSWORD=""
REDIS_DB=0
IDEMPOTENCE_TTL="24h"
SANDBOX_TTL="3m"
VERBOSE=false
RAW_OUTPUT=false
CLIENT_ONLY=false
DEV_LOGGING=true
BUILD_SANDBOX=true
MOCK_MODE=false
NAMESPACE="sandbox"
SANDBOX_IMAGE="mock-sandbox:latest"

# Array to keep track of background processes
declare -a PIDS=()

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --no-build)
      BUILD=false
      shift
      ;;
    --release)
      RELEASE_SANDBOX=true
      shift
      ;;
    --retain)
      RETAIN_SANDBOX=true
      shift
      ;;
    --no-redis)
      USE_REDIS=false
      shift
      ;;
    --verbose)
      VERBOSE=true
      shift
      ;;
    --raw)
      RAW_OUTPUT=true
      shift
      ;;
    --client-only)
      CLIENT_ONLY=true
      shift
      ;;
    --no-build-sandbox)
      BUILD_SANDBOX=false
      shift
      ;;
    --mock)
      MOCK_MODE=true
      shift
      ;;
    --help)
      echo "K8s Scheduler Demo"
      echo
      echo "Usage: ./demo.sh [options]"
      echo
      echo "Options:"
      echo "  --no-build        Skip the build step"
      echo "  --release         Also demonstrate releasing the sandbox"
      echo "  --retain          Demonstrate retaining the sandbox to extend its expiration"
      echo "  --no-redis        Disable Redis for idempotence (default: enabled)"
      echo "  --verbose         Enable verbose logging"
      echo "  --raw             Output raw JSON responses"
      echo "  --client-only     Run only the demo client without starting services"
      echo "  --no-build-sandbox Skip building the sandbox Docker image"
      echo "  --mock            Enable mock Kubernetes mode (default: disabled)"
      echo "  --help            Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help to see available options"
      exit 1
      ;;
  esac
done

# Check for required dependencies
check_dependency() {
  if ! command -v $1 &> /dev/null; then
    log_error "$1 is required but not found. Please install it."
    exit 1
  fi
}

# Check for jq which we need for JSON parsing
check_dependency jq

# Log functions - simplified to reduce verbosity
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Cleanup function to ensure all processes are terminated
cleanup() {
  if [ "$CLIENT_ONLY" = false ]; then
    log_info "Cleaning up resources..."
    
    # Kill all background processes in reverse order
    for ((i=${#PIDS[@]}-1; i>=0; i--)); do
      pid=${PIDS[$i]}
      if ps -p $pid > /dev/null; then
        kill $pid 2>/dev/null || true
        wait $pid 2>/dev/null || true
      fi
    done
  fi
}

# Register the cleanup function for various signals
trap cleanup EXIT INT TERM

# Build the demo command arguments
build_demo_args() {
  local demo_args=""

  if [ "$RETAIN_SANDBOX" = true ]; then
    demo_args="$demo_args --retain --retain-count=$RETAIN_COUNT --retain-interval=$RETAIN_INTERVAL"
  fi

  if [ "$RELEASE_SANDBOX" = true ]; then
    demo_args="$demo_args --release"
  fi

  if [ "$VERBOSE" = true ]; then
    demo_args="$demo_args --verbose"
  fi

  if [ "$RAW_OUTPUT" = true ]; then
    demo_args="$demo_args --raw"
  fi

  echo "$demo_args"
}

# Function to check if a service is ready - simplified logging
check_service_port() {
  local port=$1
  local service_name=$2
  local max_attempts=$3
  local attempt=1
  local last_pid=${PIDS[${#PIDS[@]}-1]}
  
  # Only log after waiting for a while (5 attempts)
  local initial_wait=5
  
  while ! nc -z localhost $port >/dev/null 2>&1; do
    if [ $attempt -ge $max_attempts ] || ! ps -p $last_pid > /dev/null; then
      log_error "$service_name failed to start!"
      return 1
    fi
    
    # Only show waiting message after initial wait period
    if [ $attempt -eq $initial_wait ]; then
      log_info "Waiting for $service_name on port $port..."
    fi
    
    # Only show progress every 5 attempts after initial wait
    if [ $attempt -gt $initial_wait ] && [ $((attempt % 5)) -eq 0 ]; then
      log_info "Still waiting for $service_name (attempt $attempt/$max_attempts)..."
    fi
    
    sleep 1
    ((attempt++))
  done
  
  log_success "$service_name is ready!"
  return 0
}

# Check if Redis is already running and use it if available
check_redis_running() {
    if redis-cli -p $REDIS_PORT ping > /dev/null 2>&1; then
        log_info "Using existing Redis instance on port $REDIS_PORT"
        return 0
    else
        return 1
    fi
}

# Start Redis if not already running
start_redis() {
    if check_redis_running; then
        return 0
    fi

    log_info "Starting Redis on port $REDIS_PORT..."
    redis-server --port $REDIS_PORT &
    REDIS_PID=$!
    PIDS+=($REDIS_PID)

    # Wait for Redis to be ready
    local max_attempts=10
    local attempt=1
    while ! redis-cli -p $REDIS_PORT ping > /dev/null 2>&1; do
        if [ $attempt -ge $max_attempts ]; then
            log_error "Redis failed to start after $max_attempts attempts"
            return 1
        fi
        log_info "Waiting for Redis to be ready (attempt $attempt/$max_attempts)..."
        sleep 1
        ((attempt++))
    done
    log_success "Redis is ready!"
    return 0
}

# Build the project if not skipped
if [ "$BUILD" = true ]; then
  log_info "Building the project..."
  
  if [ "$CLIENT_ONLY" = true ]; then
    # Just build the demo client if only running the client
    go build -o bin/demo ./cmd/demo || { log_error "Demo client build failed!"; exit 1; }
  else
    # Build everything for the full demo
    make build || { log_error "Build failed!"; exit 1; }
  fi
else
  log_info "Skipping build step..."
fi

# Build and load the sandbox Docker image if needed
if [ "$BUILD_SANDBOX" = true ] && [ "$CLIENT_ONLY" = false ]; then
  log_info "Building sandbox Docker image..."
  
  # Check if Docker is running
  if ! docker info > /dev/null 2>&1; then
    log_error "Docker is not running. Please start Docker Desktop and try again."
    exit 1
  fi
  
  # Build the sandbox Docker image
  cd mock-sandbox
  docker build -q -t $SANDBOX_IMAGE . || { log_error "Sandbox Docker build failed!"; exit 1; }
  cd ..
  
  # Check if we're using Docker Desktop Kubernetes
  if kubectl config current-context | grep -q "docker-desktop"; then
    log_info "Loading sandbox image into Kubernetes cluster..."
    
    # Create the namespace if it doesn't exist
    kubectl get namespace $NAMESPACE > /dev/null 2>&1 || kubectl create namespace $NAMESPACE
    
  else
    log_warning "Not using Docker Desktop Kubernetes. Make sure the sandbox image is available in your cluster."
  fi
fi

if [ "$CLIENT_ONLY" = true ]; then
  # When running in client-only mode, just verify services are running
  if ! nc -z localhost $SCHEDULER_PORT >/dev/null 2>&1; then
    log_warning "Scheduler service does not appear to be running on port $SCHEDULER_PORT!"
    log_warning "The demo might fail if the service is not available."
  fi
  
  # Run the demo client
  log_info "Running the demo client in standalone mode..."
  DEMO_ARGS=$(build_demo_args)
  ./bin/demo $DEMO_ARGS
  
  exit 0
fi

# If we get here, we're running the full demo with services

# Check if ports are in use
check_port() {
    if lsof -i :$1 > /dev/null 2>&1; then
        echo "Port $1 is already in use. Please free up the port and try again."
        exit 1
    fi
}

check_port $SCHEDULER_PORT
check_port $DEMO_PORT

# Remove the Redis port check since we'll use existing Redis if available
if [ "$CLIENT_ONLY" = false ]; then
    # Start Redis if needed
    if ! start_redis; then
        log_error "Failed to start Redis. Exiting..."
        exit 1
    fi

    # Start Scheduler service
    log_info "Starting Scheduler service..."
    SCHEDULER_ARGS="--port $SCHEDULER_PORT --namespace $NAMESPACE"
    if [ "$MOCK_MODE" = true ]; then
      SCHEDULER_ARGS="$SCHEDULER_ARGS --mock"
    fi
    if [ "$DEV_LOGGING" = true ]; then
      SCHEDULER_ARGS="$SCHEDULER_ARGS --dev-logging"
    fi
    ./bin/scheduler $SCHEDULER_ARGS &
    SCHEDULER_PID=$!
    PIDS+=($SCHEDULER_PID)

    # Wait for Scheduler service to be ready
    check_service_port $SCHEDULER_PORT "Scheduler service" $HEALTH_CHECK_TIMEOUT || exit 1
fi

# Run the demo client
log_info "Running the demo client..."
DEMO_ARGS=$(build_demo_args)
./bin/demo $DEMO_ARGS

log_info "Press ENTER to stop the services and exit..."
read

# cleanup function will be called automatically on exit
exit 0 