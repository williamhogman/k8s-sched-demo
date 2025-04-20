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

# Construct scheduler command with Redis params if enabled
SCHEDULER_CMD="./bin/scheduler --mock=false --port=$SCHEDULER_PORT --sandbox-ttl=$SANDBOX_TTL"
if [ "$USE_REDIS" = true ]; then
  REDIS_URI="redis://:$REDIS_PASSWORD@$REDIS_ADDR/$REDIS_DB"
  SCHEDULER_CMD="$SCHEDULER_CMD --idempotence=redis --redis-uri=$REDIS_URI --idempotence-ttl=$IDEMPOTENCE_TTL"
  log_info "Scheduler will use Redis at $REDIS_ADDR for idempotence with TTL=$IDEMPOTENCE_TTL and sandbox TTL=$SANDBOX_TTL"
else
  SCHEDULER_CMD="$SCHEDULER_CMD --idempotence=memory"
  log_info "Scheduler will run with in-memory idempotence store and sandbox TTL=$SANDBOX_TTL"
fi

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

# Run the API request using xh instead of client binary
log_info "Making API request to get a sandbox..."

SELECTOR_URL="http://localhost:$GLOBAL_PORT/selector.ClusterSelector/GetSandbox"

# Generate a unique idempotenceKey using timestamp
IDEMPOTENCE_KEY="demo-$(date +%s)"

# Store first API call results to demonstrate idempotence
log_info "First API call with idempotence key: $IDEMPOTENCE_KEY"

# Prepare the request payload with jq
REQUEST_PAYLOAD=$(jq -n \
  --arg key "$IDEMPOTENCE_KEY" \
  '{idempotenceKey: $key, metadata: {purpose: "demo", owner: "user"}}')

# Using xh to make the Connect RPC call
RESPONSE=$(xh POST $SELECTOR_URL \
  Content-Type:application/json \
  --raw "$REQUEST_PAYLOAD" \
  --pretty=none) || { log_error "API request failed!"; exit 1; }

# Pretty print the response JSON
echo "$RESPONSE" | jq '.'

# Extract values using jq
if [[ $(echo "$RESPONSE" | jq 'has("sandboxId")') == "true" ]]; then
  SANDBOX_ID=$(echo "$RESPONSE" | jq -r '.sandboxId')
  CLUSTER_ID=$(echo "$RESPONSE" | jq -r '.clusterId')
  log_success "Sandbox created successfully with ID: $SANDBOX_ID on cluster: $CLUSTER_ID"
  
  # If Redis is enabled, demonstrate idempotence
  if [ "$USE_REDIS" = true ]; then
    log_info "Waiting 2 seconds before making identical request to demonstrate idempotence..."
    sleep 1
    
    log_info "Second API call with same idempotence key (should return same sandbox):"
    
    # Second call with same idempotence key
    SECOND_RESPONSE=$(xh POST $SELECTOR_URL \
      Content-Type:application/json \
      --raw "$REQUEST_PAYLOAD" \
      --pretty=none) || { log_error "Second API request failed!"; exit 1; }
    
    # Pretty print the second response JSON
    echo "$SECOND_RESPONSE" | jq '.'
    
    # Extract values from second response using jq
    if [[ $(echo "$SECOND_RESPONSE" | jq 'has("sandboxId")') == "true" ]]; then
      SECOND_SANDBOX_ID=$(echo "$SECOND_RESPONSE" | jq -r '.sandboxId')
      SECOND_CLUSTER_ID=$(echo "$SECOND_RESPONSE" | jq -r '.clusterId')
      
      # Check if it's the same sandbox
      if [[ "$SANDBOX_ID" == "$SECOND_SANDBOX_ID" ]]; then
        log_success "Idempotence verified: Second request returned the same sandbox ID ($SECOND_SANDBOX_ID)"
        
        # Log cluster ID info separately
        if [[ "$CLUSTER_ID" != "$SECOND_CLUSTER_ID" ]]; then
          log_info "Note: Different cluster IDs returned ($CLUSTER_ID vs $SECOND_CLUSTER_ID), which is fine for demo purposes"
        fi
      else
        log_warning "Idempotence may not be working correctly: Got different sandbox IDs ($SANDBOX_ID vs $SECOND_SANDBOX_ID)"
      fi
    else
      log_error "Failed to extract sandbox ID from second response"
    fi
  fi
  
  # If retain option is enabled, demonstrate retaining the sandbox
  if [ "$RETAIN_SANDBOX" = true ]; then
    log_info "Demonstrating sandbox retention (extending expiration)..."
    RETAIN_URL="http://localhost:$GLOBAL_PORT/selector.ClusterSelector/RetainSandbox"
    
    # Prepare retain payload with jq
    RETAIN_PAYLOAD=$(jq -n \
      --arg sid "$SANDBOX_ID" \
      --arg cid "$CLUSTER_ID" \
      '{sandboxId: $sid, clusterId: $cid}')
    
    # Retain the sandbox multiple times to demonstrate extension
    for ((i=1; i<=$RETAIN_COUNT; i++)); do
      log_info "Retain operation $i of $RETAIN_COUNT (waiting $RETAIN_INTERVAL seconds between operations)..."
      
      # Using xh to make the Connect RPC call to retain the sandbox
      RETAIN_RESPONSE=$(xh POST $RETAIN_URL \
        Content-Type:application/json \
        --raw "$RETAIN_PAYLOAD" \
        --pretty=none) || { log_error "Retain API request $i failed!"; continue; }
      
      # Pretty print the retain response
      echo "$RETAIN_RESPONSE" | jq '.'
      
      # Check if the retain was successful using jq
      if [[ $(echo "$RETAIN_RESPONSE" | jq '.success') == "true" ]]; then
        EXPIRATION_TIME=$(echo "$RETAIN_RESPONSE" | jq -r '.expirationTime')
        
        # Format expiration time in a human-readable format, safe for all systems
        EXPIRATION_READABLE="$(date -u -d @${EXPIRATION_TIME} "+%Y-%m-%d %H:%M:%S UTC" 2>/dev/null || date -u -r ${EXPIRATION_TIME} "+%Y-%m-%d %H:%M:%S UTC" 2>/dev/null || echo "timestamp: ${EXPIRATION_TIME}")"
        
        log_success "Sandbox $SANDBOX_ID successfully retained! New expiration: $EXPIRATION_READABLE"
      else
        ERROR_MESSAGE=$(echo "$RETAIN_RESPONSE" | jq -r '.error // "Unknown error"')
        log_error "Failed to retain sandbox: $ERROR_MESSAGE"
        break
      fi
      
      # Wait between retain operations if not the last one
      if [ $i -lt $RETAIN_COUNT ]; then
        sleep $RETAIN_INTERVAL
      fi
    done
  fi
  
  # If release option is enabled, release the sandbox
  if [ "$RELEASE_SANDBOX" = true ]; then
    log_info "Waiting 3 seconds before releasing the sandbox..."
    sleep 3
    
    log_info "Releasing the sandbox..."
    RELEASE_URL="http://localhost:$GLOBAL_PORT/selector.ClusterSelector/ReleaseSandbox"
    
    # Prepare release payload with jq
    RELEASE_PAYLOAD=$(jq -n \
      --arg sid "$SANDBOX_ID" \
      --arg cid "$CLUSTER_ID" \
      '{sandboxId: $sid, clusterId: $cid}')
    
    # Using xh to make the Connect RPC call to release the sandbox
    RELEASE_RESPONSE=$(xh POST $RELEASE_URL \
      Content-Type:application/json \
      --raw "$RELEASE_PAYLOAD" \
      --pretty=none) || { log_error "Release API request failed!"; exit 1; }
    
    # Pretty print the release response
    echo "$RELEASE_RESPONSE" | jq '.'
    
    # Check if the release was successful using jq
    if [[ $(echo "$RELEASE_RESPONSE" | jq '.success') == "true" ]]; then
      log_success "Sandbox $SANDBOX_ID successfully released!"
    else
      ERROR_MESSAGE=$(echo "$RELEASE_RESPONSE" | jq -r '.error // "Unknown error"')
      log_error "Failed to release sandbox: $ERROR_MESSAGE"
    fi
  fi
else
  log_error "Failed to extract sandbox ID from response"
fi

log_info "Press ENTER to stop the services and exit..."
read

# cleanup function will be called automatically on exit
exit 0 