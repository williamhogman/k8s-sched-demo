#!/bin/bash
set -e

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
SANDBOX_TTL="15m"
VERBOSE=false
RAW_OUTPUT=false
CLIENT_ONLY=false
NAMESPACE="sandbox"
DEV_LOGGING=true

# Array to keep track of background processes
declare -a PIDS=()

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --no-build)
      BUILD=false
      shift
      ;;
    --scheduler-port)
      SCHEDULER_PORT="$2"
      shift 2
      ;;
    --global-port)
      GLOBAL_PORT="$2"
      shift 2
      ;;
    --timeout)
      HEALTH_CHECK_TIMEOUT="$2"
      shift 2
      ;;
    --release)
      RELEASE_SANDBOX=true
      shift
      ;;
    --retain)
      RETAIN_SANDBOX=true
      shift
      ;;
    --retain-count)
      RETAIN_COUNT="$2"
      shift 2
      ;;
    --retain-interval)
      RETAIN_INTERVAL="$2"
      shift 2
      ;;
    --no-redis)
      USE_REDIS=false
      shift
      ;;
    --redis-port)
      REDIS_PORT="$2"
      REDIS_ADDR="localhost:$REDIS_PORT"
      shift 2
      ;;
    --redis-addr)
      REDIS_ADDR="$2"
      shift 2
      ;;
    --redis-password)
      REDIS_PASSWORD="$2"
      shift 2
      ;;
    --redis-db)
      REDIS_DB="$2"
      shift 2
      ;;
    --idempotence-ttl)
      IDEMPOTENCE_TTL="$2"
      shift 2
      ;;
    --sandbox-ttl)
      SANDBOX_TTL="$2"
      shift 2
      ;;
    --namespace)
      NAMESPACE="$2"
      shift 2
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
    --dev-logging)
      DEV_LOGGING=true
      shift
      ;;
    --help)
      echo "K8s Scheduler Demo"
      echo
      echo "Usage: ./demo.sh [options]"
      echo
      echo "Options:"
      echo "  --no-build        Skip the build step"
      echo "  --scheduler-port  Set custom scheduler port (default: 50052)"
      echo "  --global-port     Set custom global scheduler port (default: 50051)"
      echo "  --timeout         Set service health check timeout in seconds (default: 10)"
      echo "  --release         Also demonstrate releasing the sandbox"
      echo "  --retain          Demonstrate retaining the sandbox to extend its expiration"
      echo "  --retain-count    Number of times to retain the sandbox (default: 3)"
      echo "  --retain-interval Seconds between retain operations (default: 5)"
      echo "  --no-redis        Disable Redis for idempotence (default: enabled)"
      echo "  --redis-port      Set Redis port (default: 6379)"
      echo "  --redis-addr      Set Redis address (default: localhost:6379)"
      echo "  --redis-password  Set Redis password (default: empty)"
      echo "  --redis-db        Set Redis database number (default: 0)"
      echo "  --idempotence-ttl Set TTL for idempotence keys (default: 24h)"
      echo "  --sandbox-ttl     Set TTL for sandboxes (default: 15m)"
      echo "  --namespace       Kubernetes namespace to use for sandboxes (default: sandbox)"
      echo "  --verbose         Enable verbose logging"
      echo "  --raw             Output raw JSON responses"
      echo "  --client-only     Run only the demo client without starting services"
      echo "  --dev-logging     Enable development logging mode"
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

# Log functions
log_info() { echo -e "${BLUE}[$(date "+%Y-%m-%d %H:%M:%S") INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[$(date "+%Y-%m-%d %H:%M:%S") SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[$(date "+%Y-%m-%d %H:%M:%S") WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[$(date "+%Y-%m-%d %H:%M:%S") ERROR]${NC} $1"; }

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
    
    log_success "All processes have been terminated."
  fi
}

# Register the cleanup function for various signals
trap cleanup EXIT INT TERM

# Build the demo command arguments
build_demo_args() {
  local demo_args="--global-port=$GLOBAL_PORT"

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

# Function to check if a service is ready
check_service_port() {
  local port=$1
  local service_name=$2
  local max_attempts=$3
  local attempt=1
  local last_pid=${PIDS[${#PIDS[@]}-1]}
  
  log_info "Waiting for $service_name to be ready on port $port..."
  
  while ! nc -z localhost $port >/dev/null 2>&1; do
    if [ $attempt -ge $max_attempts ] || ! ps -p $last_pid > /dev/null; then
      log_error "$service_name failed to start!"
      return 1
    fi
    
    log_info "Waiting for $service_name to start (attempt $attempt/$max_attempts)..."
    sleep 1
    ((attempt++))
  done
  
  log_success "$service_name is ready on port $port!"
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
  
  log_success "Build completed successfully!"
else
  log_info "Skipping build step..."
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
      log_success "Redis started successfully on port $REDIS_PORT"
    else
      log_error "Failed to start Redis. Please make sure Redis is installed and available."
      exit 1
    fi
  fi
  
  # Verify Redis connectivity with ping
  if redis-cli -p $REDIS_PORT ping > /dev/null 2>&1; then
    log_success "Redis connectivity verified with PING"
  else
    log_warning "Redis server is running but ping failed. Continuing anyway..."
  fi
fi

# Start the Scheduler service
log_info "Starting the Scheduler service on port $SCHEDULER_PORT..."

# Construct scheduler command with Redis params if enabled
SCHEDULER_CMD="./bin/scheduler --mock=false --port=$SCHEDULER_PORT --sandbox-ttl=$SANDBOX_TTL --namespace=$NAMESPACE"
if [ "$DEV_LOGGING" = true ]; then
  SCHEDULER_CMD="$SCHEDULER_CMD --dev-logging"
  log_info "Development logging enabled"
fi

if [ "$USE_REDIS" = true ]; then
  REDIS_URI="redis://:$REDIS_PASSWORD@$REDIS_ADDR/$REDIS_DB"
  SCHEDULER_CMD="$SCHEDULER_CMD --idempotence=redis --redis-uri=$REDIS_URI --idempotence-ttl=$IDEMPOTENCE_TTL"
  log_info "Scheduler will use Redis at $REDIS_ADDR for idempotence with TTL=$IDEMPOTENCE_TTL and sandbox TTL=$SANDBOX_TTL"
else
  SCHEDULER_CMD="$SCHEDULER_CMD --idempotence=memory"
  log_info "Scheduler will run with in-memory idempotence store and sandbox TTL=$SANDBOX_TTL"
fi

# Log the namespace being used
log_info "Using Kubernetes namespace: $NAMESPACE"

# Execute scheduler command
$SCHEDULER_CMD &
PIDS+=($!)

# Check if the scheduler service is ready
check_service_port $SCHEDULER_PORT "Scheduler service" $HEALTH_CHECK_TIMEOUT || exit 1

# Start the Global Scheduler service
log_info "Starting the Global Scheduler service on port $GLOBAL_PORT..."
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