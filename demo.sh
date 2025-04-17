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

# Log functions
log_info() { echo -e "${BLUE}[$(date "+%Y-%m-%d %H:%M:%S") INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[$(date "+%Y-%m-%d %H:%M:%S") SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[$(date "+%Y-%m-%d %H:%M:%S") WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[$(date "+%Y-%m-%d %H:%M:%S") ERROR]${NC} $1"; }

# Cleanup function to ensure all processes are terminated
cleanup() {
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
}

# Register the cleanup function for various signals
trap cleanup EXIT INT TERM

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

# Check if ports are already in use
if nc -z localhost $SCHEDULER_PORT >/dev/null 2>&1; then
  log_error "Port $SCHEDULER_PORT is already in use!"
  exit 1
fi

if nc -z localhost $GLOBAL_PORT >/dev/null 2>&1; then
  log_error "Port $GLOBAL_PORT is already in use!"
  exit 1
fi

# Build the project if not skipped
if [ "$BUILD" = true ]; then
  log_info "Building the project..."
  make build || { log_error "Build failed!"; exit 1; }
  log_success "Build completed successfully!"
else
  log_info "Skipping build step..."
fi

# Start the Scheduler service
log_info "Starting the Scheduler service on port $SCHEDULER_PORT..."
./bin/scheduler --mock=false --port=$SCHEDULER_PORT &
PIDS+=($!)

# Check if the scheduler service is ready
check_service_port $SCHEDULER_PORT "Scheduler service" $HEALTH_CHECK_TIMEOUT || exit 1

# Start the Global Scheduler service
log_info "Starting the Global Scheduler service on port $GLOBAL_PORT..."
./bin/global-scheduler --port=$GLOBAL_PORT &
PIDS+=($!)

# Check if the global scheduler service is ready
check_service_port $GLOBAL_PORT "Global Scheduler service" $HEALTH_CHECK_TIMEOUT || exit 1

# Run the API request using xh instead of client binary
log_info "Making API request to get a sandbox..."

SELECTOR_URL="http://localhost:$GLOBAL_PORT/selector.ClusterSelector/GetSandbox"

# Using xh to make the Connect RPC call
xh POST $SELECTOR_URL \
  Content-Type:application/json \
  --raw '{"sandboxId":"123","configuration":{}}' \
  --pretty=format || { log_error "API request failed!"; exit 1; }

log_info "Press ENTER to stop the services and exit..."
read

# cleanup function will be called automatically on exit
exit 0 