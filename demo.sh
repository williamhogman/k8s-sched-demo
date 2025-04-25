#!/bin/bash
set -euo pipefail

# ================================================================
# K8s Scheduler Demo - Kubernetes Deployment Version
# 
# This script demonstrates the K8s scheduler demo system running in Kubernetes
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
RELEASE_SANDBOX=false
RETAIN_SANDBOX=false
RETAIN_COUNT=3
RETAIN_INTERVAL=5
VERBOSE=false
RAW_OUTPUT=false
CLIENT_ONLY=false
NAMESPACE="sandbox"
SANDBOX_IMAGE="mock-sandbox:latest"
SCHEDULER_IMAGE="k8s-sched-demo-scheduler:latest"
CONTEXT="rancher-desktop"

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
    --context)
      CONTEXT="$2"
      shift 2
      ;;
    --help)
      echo "K8s Scheduler Demo - Kubernetes Deployment Version"
      echo
      echo "Usage: ./demo.sh [options]"
      echo
      echo "Options:"
      echo "  --no-build        Skip the build step"
      echo "  --release         Also demonstrate releasing the sandbox"
      echo "  --retain          Demonstrate retaining the sandbox to extend its expiration"
      echo "  --verbose         Enable verbose logging"
      echo "  --raw             Output raw JSON responses"
      echo "  --client-only     Run only the demo client without deploying services"
      echo "  --context         Kubernetes context to use (default: rancher-desktop)"
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

# Check required dependencies
check_dependency kubectl
check_dependency docker
check_dependency jq

# Log functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Wait for a Kubernetes deployment to be ready
wait_for_deployment() {
  local namespace=$1
  local deployment=$2
  local timeout=$3
  local interval=5
  local elapsed=0

  log_info "Waiting for deployment $deployment to be ready..."
  
  while [ $elapsed -lt $timeout ]; do
    if kubectl get deployment -n $namespace $deployment -o jsonpath='{.status.readyReplicas}' 2>/dev/null | grep -q 1; then
      log_success "Deployment $deployment is ready!"
      return 0
    fi
    
    sleep $interval
    elapsed=$((elapsed + interval))
    
    if [ $elapsed -ge $timeout ]; then
      log_error "Timeout waiting for deployment $deployment"
      return 1
    fi
    
    log_info "Still waiting for deployment $deployment... ($elapsed/$timeout seconds)"
  done
  
  return 1
}

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

# Function to set up port forwarding
setup_port_forwarding() {
  local namespace=$1
  local service=$2
  local local_port=$3
  local service_port=$4
  
  log_info "Setting up port-forwarding from localhost:$local_port to $service:$service_port"
  kubectl port-forward -n $namespace svc/$service $local_port:$service_port &
  PORT_FWD_PID=$!
  
  # Wait for port-forwarding to be established
  sleep 2
  
  # Check if port-forwarding is working
  if ! nc -z localhost $local_port &>/dev/null; then
    log_error "Port-forwarding failed"
    kill $PORT_FWD_PID 2>/dev/null || true
    return 1
  fi
  
  return 0
}

# Cleanup function
cleanup() {
  log_info "Cleaning up resources..."
  
  # Kill port forwarding if active
  if [ -n "${PORT_FWD_PID:-}" ]; then
    kill $PORT_FWD_PID 2>/dev/null || true
    wait $PORT_FWD_PID 2>/dev/null || true
  fi
  
  if [ "$CLIENT_ONLY" = false ]; then
    log_info "To delete the deployed resources, run:"
    log_info "kubectl delete -f k8s/scheduler-deployment.yaml"
    log_info "kubectl delete -f k8s/redis-deployment.yaml"
    log_info "kubectl delete -f k8s/sandbox-namespace.yaml"
  fi
}

# Register the cleanup function for various signals
trap cleanup EXIT INT TERM

# Ensure we're using the right Kubernetes context
kubectl config use-context $CONTEXT

# Check if the namespace exists, create it if not
if ! kubectl get namespace $NAMESPACE &>/dev/null; then
  log_info "Creating namespace $NAMESPACE"
  kubectl apply -f k8s/sandbox-namespace.yaml
fi

# Build and deploy if not in client-only mode
if [ "$CLIENT_ONLY" = false ]; then
  if [ "$BUILD" = true ]; then
    # Build the scheduler image
    log_info "Building scheduler Docker image..."
    docker build -t $SCHEDULER_IMAGE -f Dockerfile.scheduler .
    
    # Build the sandbox image
    log_info "Building mock-sandbox Docker image..."
    docker build -t $SANDBOX_IMAGE mock-sandbox
  fi
  
  # Deploy Redis
  log_info "Deploying Redis to Kubernetes..."
  kubectl apply -f k8s/redis-deployment.yaml
  wait_for_deployment $NAMESPACE redis 60 || { log_error "Redis deployment failed"; exit 1; }
  
  # Deploy the scheduler
  log_info "Deploying scheduler to Kubernetes..."
  kubectl apply -f k8s/scheduler-deployment.yaml
  wait_for_deployment $NAMESPACE scheduler 60 || { log_error "Scheduler deployment failed"; exit 1; }
fi

# Set up port forwarding to the scheduler service
setup_port_forwarding $NAMESPACE scheduler $SCHEDULER_PORT $SCHEDULER_PORT || { log_error "Port forwarding failed"; exit 1; }

# Build the demo client
if [ "$BUILD" = true ]; then
  log_info "Building the demo client..."
  go build -o bin/demo ./cmd/demo || { log_error "Demo client build failed!"; exit 1; }
fi

# Run the demo client
log_info "Running the demo client..."
DEMO_ARGS=$(build_demo_args)
./bin/demo $DEMO_ARGS --scheduler-host=localhost --scheduler-port=$SCHEDULER_PORT

log_info "Demo complete!"
exit 0 