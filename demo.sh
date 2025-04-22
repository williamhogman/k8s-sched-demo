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
SCHEDULER_PORT=50052
GLOBAL_PORT=50051
EVENTS_PORT=50053
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
NAMESPACE="sandbox"
DEV_LOGGING=true
ENABLE_EVENTS=true
BUILD_SANDBOX=true
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

# Build the project if not skipped
if [ "$BUILD" = true ]; then
  log_info "Building the project..."
  
  if [ "$CLIENT_ONLY" = true ]; then
    # Just build the demo client if only running the client
    go build -o bin/demo ./cmd/demo || { log_error "Demo client build failed!"; exit 1; }
  else
    # Build everything for the full demo
    make build || { log_error "Build failed!"; exit 1; }
    
    # Build the test-events service if events are enabled
    if [ "$ENABLE_EVENTS" = true ]; then
      go build -o bin/test-events ./cmd/test-events || { log_error "Test events build failed!"; exit 1; }
    fi
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
  if ! nc -z localhost $GLOBAL_PORT >/dev/null 2>&1; then
    log_warning "Global scheduler service does not appear to be running on port $GLOBAL_PORT!"
    log_warning "The demo might fail if the service is not available."
  fi
  
  # Run the demo client
  log_info "Running the demo client in standalone mode..."
  DEMO_ARGS=$(build_demo_args)
  ./bin/demo $DEMO_ARGS
  
  exit 0
fi

# If we get here, we're running the full demo with services

# Check if ports are already in use
if nc -z localhost $SCHEDULER_PORT >/dev/null 2>&1; then
  log_error "Port $SCHEDULER_PORT is already in use!"
  exit 1
fi

if nc -z localhost $GLOBAL_PORT >/dev/null 2>&1; then
  log_error "Port $GLOBAL_PORT is already in use!"
  exit 1
fi

if [ "$ENABLE_EVENTS" = true ] && nc -z localhost $EVENTS_PORT >/dev/null 2>&1; then
  log_error "Port $EVENTS_PORT is already in use!"
  exit 1
fi

# Check Redis and start if needed
if [ "$USE_REDIS" = true ]; then
  log_info "Checking Redis server..."
  if nc -z localhost $REDIS_PORT >/dev/null 2>&1; then
    log_success "Redis is already running on port $REDIS_PORT"
  else
    log_info "Starting Redis server on port $REDIS_PORT..."
    redis-server --port $REDIS_PORT --daemonize yes
    sleep 1
    
    # Check if Redis started successfully
    if nc -z localhost $REDIS_PORT >/dev/null 2>&1; then
      log_success "Redis started successfully"
    else
      log_error "Failed to start Redis. Please make sure Redis is installed and available."
      exit 1
    fi
  fi
  
  # Verify Redis connectivity with ping
  if redis-cli -p $REDIS_PORT ping > /dev/null 2>&1; then
    # Only log if verbose mode is enabled
    if [ "$VERBOSE" = true ]; then
      log_success "Redis connectivity verified with PING"
    fi
  else
    log_warning "Redis server is running but ping failed. Continuing anyway..."
  fi
fi

# Start the event service if enabled
if [ "$ENABLE_EVENTS" = true ]; then
  log_info "Starting the Events service..."
  ./bin/test-events --port=$EVENTS_PORT &
  PIDS+=($!)

  # Check if the events service is ready
  check_service_port $EVENTS_PORT "Events service" $HEALTH_CHECK_TIMEOUT || exit 1
fi

# Start the Scheduler service
log_info "Starting the Scheduler service..."

# Construct scheduler command with Redis params if enabled
SCHEDULER_CMD="./bin/scheduler --mock=false --port=$SCHEDULER_PORT --sandbox-ttl=$SANDBOX_TTL --namespace=$NAMESPACE --sandbox-image=$SANDBOX_IMAGE"
if [ "$DEV_LOGGING" = true ]; then
  SCHEDULER_CMD="$SCHEDULER_CMD --dev-logging"
fi

if [ "$USE_REDIS" = true ]; then
  REDIS_URI="redis://:$REDIS_PASSWORD@$REDIS_ADDR/$REDIS_DB"
  SCHEDULER_CMD="$SCHEDULER_CMD --idempotence=redis --redis-uri=$REDIS_URI --idempotence-ttl=$IDEMPOTENCE_TTL"
fi

# Add event broadcasting configuration if enabled
if [ "$ENABLE_EVENTS" = true ]; then
  SCHEDULER_CMD="$SCHEDULER_CMD --event-broadcast=true --event-endpoint=localhost:$EVENTS_PORT"
fi

# Execute scheduler command
$SCHEDULER_CMD &
PIDS+=($!)

# Check if the scheduler service is ready
check_service_port $SCHEDULER_PORT "Scheduler service" $HEALTH_CHECK_TIMEOUT || exit 1

# Start the Global Scheduler service
log_info "Starting the Global Scheduler service..."
./bin/global-scheduler --port=$GLOBAL_PORT &
PIDS+=($!)

# Check if the global scheduler service is ready
check_service_port $GLOBAL_PORT "Global Scheduler service" $HEALTH_CHECK_TIMEOUT || exit 1

# Run the demo client
log_info "Running the demo client..."
DEMO_ARGS=$(build_demo_args)
./bin/demo $DEMO_ARGS

log_info "Press ENTER to stop the services and exit..."
read

# cleanup function will be called automatically on exit
exit 0 