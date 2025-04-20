#!/bin/bash

# Simple wrapper script to run the demo with Redis configuration
# See demo.sh for full options

echo "Running scheduler demo with Redis support..."

# Customizable Redis parameters with defaults
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_ADDR="${REDIS_ADDR:-localhost:$REDIS_PORT}"
REDIS_PASSWORD="${REDIS_PASSWORD:-''}"
REDIS_DB="${REDIS_DB:-0}"
IDEMPOTENCE_TTL="${IDEMPOTENCE_TTL:-24h}"

# Construct Redis URI
if [ -z "$REDIS_URI" ]; then
  REDIS_URI="redis://:$REDIS_PASSWORD@$REDIS_ADDR/$REDIS_DB"
fi

# Execute demo.sh with Redis parameters
./demo.sh \
  --idempotence=redis \
  --redis-uri="$REDIS_URI" \
  --idempotence-ttl="$IDEMPOTENCE_TTL" \
  "$@"  # Pass any additional arguments to demo.sh

# Example usage:
#
# Basic usage:
#   ./run-scheduler-with-redis.sh
#
# With custom Redis URI:
#   REDIS_URI="redis://:password@localhost:6379/0" ./run-scheduler-with-redis.sh
#
# With Redis components:
#   REDIS_PORT=6380 REDIS_PASSWORD=secret REDIS_DB=1 ./run-scheduler-with-redis.sh
#
# With additional demo.sh options:
#   ./run-scheduler-with-redis.sh --release --scheduler-port=50055 