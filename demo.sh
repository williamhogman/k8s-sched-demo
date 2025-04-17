#!/bin/bash
set -e

# ================================================================
# K8s Scheduler Demo
# 
# This script demonstrates the K8s scheduler demo system by:
# 1. Building the project
# 2. Starting the Scheduler service with mock K8s client
# 3. Starting the Global Scheduler service
# 4. Running a client demo that requests a sandbox
# 
# Usage:
#   ./demo.sh [options]
#
# Options:
#   --no-build        Skip the build step
#   --scheduler-port  Set custom scheduler port (default: 50052)
#   --global-port     Set custom global scheduler port (default: 50051)
#   --timeout         Set service health check timeout in seconds (default: 10)
#   --help            Show this help message
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

# Timestamp function
timestamp() {
  date "+%Y-%m-%d %H:%M:%S"
}

# Log functions
log_info() {
  echo -e "${BLUE}[$(timestamp) INFO]${NC} $1"
}

log_success() {
  echo -e "${GREEN}[$(timestamp) SUCCESS]${NC} $1"
}

log_warning() {
  echo -e "${YELLOW}[$(timestamp) WARNING]${NC} $1"
}

log_error() {
  echo -e "${RED}[$(timestamp) ERROR]${NC} $1"
}

# Check if a port is already in use
is_port_in_use() {
  local port=$1
  if nc -z localhost "$port" >/dev/null 2>&1; then
    return 0  # Port is in use
  else
    return 1  # Port is free
  fi
}

# Cleanup function to ensure all processes are terminated
cleanup() {
  log_info "Cleaning up resources..."
  
  # Kill all background processes in reverse order
  for ((i=${#PIDS[@]}-1; i>=0; i--)); do
    pid=${PIDS[$i]}
    if ps -p $pid > /dev/null; then
      log_info "Stopping process with PID $pid..."
      kill $pid 2>/dev/null || true
      # Wait for process to terminate gracefully
      for j in {1..5}; do
        if ! ps -p $pid > /dev/null; then
          break
        fi
        sleep 1
      done
      # Force kill if still running
      if ps -p $pid > /dev/null; then
        log_warning "Process $pid did not terminate gracefully, force killing..."
        kill -9 $pid 2>/dev/null || true
      fi
    fi
  done
  
  log_success "All processes have been terminated."
}

# Register the cleanup function for various signals
trap cleanup EXIT INT TERM

# Function to check if a service is ready by checking a port
check_service_port() {
  local port=$1
  local service_name=$2
  local max_attempts=$3
  local attempt=1
  local last_pid=${PIDS[${#PIDS[@]}-1]}
  
  log_info "Waiting for $service_name to be ready on port $port..."
  
  while ! nc -z localhost $port >/dev/null 2>&1; do
    if [ $attempt -ge $max_attempts ]; then
      log_error "$service_name failed to start after $max_attempts attempts!"
      return 1
    fi
    
    # Check if process is still running
    if ! ps -p $last_pid > /dev/null; then
      log_error "$service_name process died unexpectedly!"
      return 1
    fi
    
    log_info "Waiting for $service_name to start (attempt $attempt/$max_attempts)..."
    sleep 1
    ((attempt++))
  done
  
  log_success "$service_name is ready and listening on port $port!"
  return 0
}

# Check if ports are already in use
if is_port_in_use $SCHEDULER_PORT; then
  log_error "Port $SCHEDULER_PORT is already in use! Please stop any running scheduler service or use --scheduler-port to specify a different port."
  exit 1
fi

if is_port_in_use $GLOBAL_PORT; then
  log_error "Port $GLOBAL_PORT is already in use! Please stop any running global scheduler service or use --global-port to specify a different port."
  exit 1
fi

# Build the project if not skipped
if [ "$BUILD" = true ]; then
  log_info "Building the project..."
  if ! make build; then
    log_error "Build failed! Check the build logs for details."
    exit 1
  fi
  log_success "Build completed successfully!"
else
  log_info "Skipping build step..."
fi

# Start the Scheduler service
log_info "Starting the Scheduler service (with mock K8s client) on port $SCHEDULER_PORT..."
./bin/scheduler --mock=false --port=$SCHEDULER_PORT &
scheduler_pid=$!
PIDS+=($scheduler_pid)

if ! ps -p $scheduler_pid > /dev/null; then
  log_error "Scheduler process failed to start!"
  exit 1
fi
log_info "Scheduler service started with PID $scheduler_pid"

# Check if the scheduler service is ready
if ! check_service_port $SCHEDULER_PORT "Scheduler service" $HEALTH_CHECK_TIMEOUT; then
  log_error "Failed to start Scheduler service. Exiting."
  exit 1
fi

# Start the Global Scheduler service
log_info "Starting the Global Scheduler service with simplified API on port $GLOBAL_PORT..."
./bin/global-scheduler --port=$GLOBAL_PORT &
global_pid=$!
PIDS+=($global_pid)

if ! ps -p $global_pid > /dev/null; then
  log_error "Global Scheduler process failed to start!"
  exit 1
fi
log_info "Global Scheduler service started with PID $global_pid"

# Check if the global scheduler service is ready
if ! check_service_port $GLOBAL_PORT "Global Scheduler service" $HEALTH_CHECK_TIMEOUT; then
  log_error "Failed to start Global Scheduler service. Exiting."
  exit 1
fi

# Run the client
log_info "Running the client to get a sandbox using the simplified API..."
log_info "This will use a single HTTP call to the Global Scheduler which handles both cluster selection and sandbox scheduling."
log_info "Resources are managed internally by the scheduler with default settings."

if ! ./bin/client --selector="http://localhost:$GLOBAL_PORT"; then
  log_error "Client execution failed!"
  exit 1
fi

log_success "Demo completed successfully!"
log_info "Press ENTER to stop the services and exit..."
read

# cleanup function will be called automatically on exit
log_info "Stopping services and cleaning up..."
exit 0 