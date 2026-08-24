#!/usr/bin/env bash
# Pull (if needed) and run the local Petstore 3 OpenAPI server in Docker.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
IMAGE="${PETSTORE_IMAGE:-context-mesh-petstore3:local}"
NAME="${PETSTORE_CONTAINER:-petstore-openapi-server}"
HOST_PORT="${PETSTORE_PORT:-8090}"
UPSTREAM="${PETSTORE_UPSTREAM:-swaggerapi/petstore3:latest}"
PLATFORM="${PETSTORE_PLATFORM:-linux/amd64}"
PULL_TIMEOUT="${PETSTORE_PULL_TIMEOUT:-120}"
CONTAINER_PORT=8080
BASE_URL="http://localhost:${HOST_PORT}/api/v3"

usage() {
  cat <<EOF
usage: $(basename "$0") [--rebuild]

  --rebuild   remove the local image/container and pull again

Env:
  PETSTORE_IMAGE         local tag (default ${IMAGE})
  PETSTORE_CONTAINER     container name (default ${NAME})
  PETSTORE_PORT          host port mapped to 8080 (default ${HOST_PORT})
  PETSTORE_UPSTREAM      image to pull (default ${UPSTREAM})
  PETSTORE_PLATFORM      pull/run platform (default ${PLATFORM})
  PETSTORE_PULL_TIMEOUT  seconds to wait for docker pull (default ${PULL_TIMEOUT})
EOF
}

registry_help() {
  cat >&2 <<EOF

docker could not pull ${UPSTREAM} (often a Docker Desktop + VPN registry hang).

Host DNS can work while the Docker VM cannot. Try:
  1. Disconnect VPN
  2. Restart Docker Desktop
  3. Re-run this script

Or skip Docker and use the hosted API:
  go run ./examples/petstore/async-order-server -petstore hosted
  go run ./examples/petstore/mcp-server -petstore hosted
EOF
}

# docker pull with a client-side timeout (macOS has no GNU timeout by default).
pull_upstream() {
  echo "pulling ${UPSTREAM} (--platform ${PLATFORM}, timeout ${PULL_TIMEOUT}s)..."
  docker pull --platform "$PLATFORM" "$UPSTREAM" &
  local pid=$!
  local i=0
  while kill -0 "$pid" 2>/dev/null; do
    i=$((i + 1))
    if [[ "$i" -ge "$PULL_TIMEOUT" ]]; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
      echo "timed out after ${PULL_TIMEOUT}s pulling ${UPSTREAM}" >&2
      registry_help
      return 1
    fi
    sleep 1
  done
  wait "$pid"
}

REBUILD=0
for arg in "$@"; do
  case "$arg" in
    -h|--help)
      usage
      exit 0
      ;;
    --rebuild)
      REBUILD=1
      ;;
    *)
      echo "unknown argument: $arg" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required (Docker Desktop or engine on PATH)" >&2
  exit 1
fi

if [[ "$REBUILD" -eq 1 ]]; then
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker image rm -f "$IMAGE" >/dev/null 2>&1 || true
fi

image_exists() {
  docker image inspect "$1" >/dev/null 2>&1
}

if image_exists "$IMAGE"; then
  echo "using existing image ${IMAGE}"
elif image_exists "$UPSTREAM"; then
  echo "tagging local ${UPSTREAM} as ${IMAGE} (skipping pull)"
  docker tag "$UPSTREAM" "$IMAGE"
else
  pull_upstream
  docker tag "$UPSTREAM" "$IMAGE"
fi

if docker ps --format '{{.Names}}' | grep -qx "$NAME"; then
  echo "container ${NAME} already running"
elif docker ps -a --format '{{.Names}}' | grep -qx "$NAME"; then
  echo "starting existing container ${NAME}"
  docker start "$NAME" >/dev/null
else
  echo "starting container ${NAME} on localhost:${HOST_PORT}"
  docker run -d --name "$NAME" --platform "$PLATFORM" \
    -p "${HOST_PORT}:${CONTAINER_PORT}" "$IMAGE" >/dev/null
fi

echo "waiting for ${BASE_URL}/openapi.json ..."
ready=0
for _ in $(seq 1 60); do
  if curl -sf "${BASE_URL}/openapi.json" >/dev/null 2>&1 \
    || curl -sf "${BASE_URL}/pet/findByStatus?status=available" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "$ready" -ne 1 ]]; then
  echo "timed out waiting for Petstore at ${BASE_URL}" >&2
  echo "logs:" >&2
  docker logs "$NAME" 2>&1 | tail -n 40 >&2 || true
  exit 1
fi

echo "Petstore 3 OpenAPI: ${BASE_URL}"
echo "Swagger UI:         http://localhost:${HOST_PORT}/"
echo "stop: docker stop ${NAME}"
