#!/bin/bash
# Demo script specifically for showing Redis-based idempotence

# Color codes for better readability
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

IDEMPOTENCE_KEY="${1:-demo-idem-$(date +%s)}"

if [ "$1" == "--help" ] || [ "$1" == "-h" ]; then
  echo "Redis Idempotence Demo"
  echo
  echo "This script demonstrates the idempotence feature using Redis."
  echo "It sends multiple identical requests with the same idempotence key"
  echo "to show that only one sandbox is created."
  echo
  echo "Usage: ./demo-idempotence.sh [idempotence_key]"
  echo
  echo "If no idempotence_key is provided, a unique one will be generated."
  exit 0
fi

# Check for required dependencies
check_dependency() {
  if ! command -v $1 &> /dev/null; then
    log_error "$1 is required but not found. Please install it."
    exit 1
  fi
}

# Check for jq which we need for JSON parsing
check_dependency jq
check_dependency redis-cli
check_dependency xh

# Confirm Redis is running
if ! redis-cli ping >/dev/null 2>&1; then
  log_error "Redis is not running. Please start Redis first."
  log_info "You can use './demo.sh' which will start Redis automatically."
  exit 1
fi

log_info "Demonstrating sandbox scheduling with idempotence"
log_info "Using idempotence key: $IDEMPOTENCE_KEY"
log_info "==============================================="

# Global Scheduler endpoint
ENDPOINT="http://localhost:50051/selector.ClusterSelector/GetSandbox"

# Prepare the request payload with jq
REQUEST_PAYLOAD=$(jq -n \
  --arg key "$IDEMPOTENCE_KEY" \
  '{idempotenceKey: $key, metadata: {purpose: "demo", owner: "user"}}')

# Make first request
log_info "Making first API request..."
RESPONSE=$(xh POST $ENDPOINT \
  Content-Type:application/json \
  --raw "$REQUEST_PAYLOAD" \
  --pretty=none) || { log_error "First API request failed!"; exit 1; }

# Pretty-print the response
echo "$RESPONSE" | jq '.'

# Extract first sandbox ID
if [[ $(echo "$RESPONSE" | jq 'has("sandboxId")') == "true" ]]; then
  SANDBOX_ID=$(echo "$RESPONSE" | jq -r '.sandboxId')
  CLUSTER_ID=$(echo "$RESPONSE" | jq -r '.clusterId')
  log_success "First request: Created sandbox with ID: $SANDBOX_ID on cluster: $CLUSTER_ID"
  
  # Verify in Redis
  REDIS_KEY="sandbox:idempotence:$IDEMPOTENCE_KEY"
  REDIS_PENDING_KEY="sandbox:idempotence:pending:$IDEMPOTENCE_KEY"
  REDIS_VALUE=$(redis-cli get "$REDIS_KEY")
  TTL=$(redis-cli ttl "$REDIS_KEY")
  PENDING_EXISTS=$(redis-cli exists "$REDIS_PENDING_KEY")
  
  log_info "Redis keys:"
  log_info "  Main key: $REDIS_KEY"
  log_info "  Pending key: $REDIS_PENDING_KEY (exists: $PENDING_EXISTS)"
  
  if [ "$REDIS_VALUE" == "$SANDBOX_ID" ]; then
    log_success "Redis verification: Key '$REDIS_KEY' correctly stores sandbox ID '$SANDBOX_ID' (TTL: $TTL seconds)"
  else
    log_error "Redis verification failed: Expected '$SANDBOX_ID', got '$REDIS_VALUE'"
  fi
else
  log_error "Failed to extract sandbox ID from response"
  exit 1
fi

# Wait briefly
sleep 2

# Make second identical request
log_info "Making second identical API request..."
RESPONSE2=$(xh POST $ENDPOINT \
  Content-Type:application/json \
  --raw "$REQUEST_PAYLOAD" \
  --pretty=none) || { log_error "Second API request failed!"; exit 1; }

# Pretty-print the response
echo "$RESPONSE2" | jq '.'

# Extract second sandbox ID
if [[ $(echo "$RESPONSE2" | jq 'has("sandboxId")') == "true" ]]; then
  SANDBOX_ID2=$(echo "$RESPONSE2" | jq -r '.sandboxId')
  CLUSTER_ID2=$(echo "$RESPONSE2" | jq -r '.clusterId')
  
  if [[ "$SANDBOX_ID" == "$SANDBOX_ID2" && "$CLUSTER_ID" == "$CLUSTER_ID2" ]]; then
    log_success "Idempotence SUCCESS: Second request returned the same sandbox ID: $SANDBOX_ID2"
  else
    log_error "Idempotence FAILED: Got different sandbox IDs: $SANDBOX_ID vs $SANDBOX_ID2"
  fi
else
  log_error "Failed to extract sandbox ID from second response"
  exit 1
fi

# Wait again and make a third request (this time send two simultaneous requests to test race condition handling)
sleep 2

log_info "Making two simultaneous API requests (testing race condition handling)..."

# Start two requests in parallel
xh POST $ENDPOINT \
  Content-Type:application/json \
  --raw "$REQUEST_PAYLOAD" \
  --pretty=none > /tmp/response3a.json &
PID1=$!

xh POST $ENDPOINT \
  Content-Type:application/json \
  --raw "$REQUEST_PAYLOAD" \
  --pretty=none > /tmp/response3b.json &
PID2=$!

# Wait for both to complete
wait $PID1 $PID2

# Check both responses
log_info "Response from first parallel request:"
cat /tmp/response3a.json | jq '.'

log_info "Response from second parallel request:"
cat /tmp/response3b.json | jq '.'

# Extract sandbox IDs from both responses
SANDBOX_ID3A=$(cat /tmp/response3a.json | jq -r '.sandboxId // ""')
SANDBOX_ID3B=$(cat /tmp/response3b.json | jq -r '.sandboxId // ""')

if [[ -n "$SANDBOX_ID3A" && -n "$SANDBOX_ID3B" ]]; then
  if [[ "$SANDBOX_ID3A" == "$SANDBOX_ID3B" && "$SANDBOX_ID" == "$SANDBOX_ID3A" ]]; then
    log_success "Race condition handling SUCCESS: Both parallel requests returned the same sandbox ID: $SANDBOX_ID3A"
    log_success "Idempotence VERIFIED: All requests returned the same sandbox ID: $SANDBOX_ID"
  else
    log_warning "Race condition handling may need improvement:"
    log_warning "  Original sandbox ID: $SANDBOX_ID"
    log_warning "  Parallel request 1:  $SANDBOX_ID3A"
    log_warning "  Parallel request 2:  $SANDBOX_ID3B"
  fi
else
  log_error "Failed to extract sandbox IDs from parallel responses"
fi

# Cleanup
rm -f /tmp/response3a.json /tmp/response3b.json

log_info "Demo completed. Your idempotence key was: $IDEMPOTENCE_KEY"
log_info "This key will remain in Redis until its TTL expires."
log_info "You can check it with: redis-cli get '$REDIS_KEY'"
log_info "You can also check if there are any pending creation markers: redis-cli get '$REDIS_PENDING_KEY'" 