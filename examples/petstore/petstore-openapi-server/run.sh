#!/usr/bin/env bash
# Build (if needed) and run the local Petstore 3 OpenAPI server in Docker.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
IMAGE="${PETSTORE_IMAGE:-context-mesh-petstore3:local}"
NAME="${PETSTORE_CONTAINER:-petstore-openapi-server}"
HOST_PORT="${PETSTORE_PORT:-8090}"
CONTAINER_PORT=8080
BASE_URL="http://localhost:${HOST_PORT}/api/v3"

usage() {
  cat <<EOF
usage: $(basename "$0") [--rebuild]

  --rebuild   remove the local image/container and build again

Env:
  PETSTORE_IMAGE      docker image tag (default ${IMAGE})
  PETSTORE_CONTAINER  container name (default ${NAME})
  PETSTORE_PORT       host port mapped to container 8080 (default ${HOST_PORT})
EOF
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

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "image ${IMAGE} not found; building (pulls swaggerapi/petstore3:unstable once)..."
  docker build -t "$IMAGE" "$ROOT"
else
  echo "using existing image ${IMAGE}"
fi

if docker ps --format '{{.Names}}' | grep -qx "$NAME"; then
  echo "container ${NAME} already running"
elif docker ps -a --format '{{.Names}}' | grep -qx "$NAME"; then
  echo "starting existing container ${NAME}"
  docker start "$NAME" >/dev/null
else
  echo "starting container ${NAME} on localhost:${HOST_PORT}"
  docker run -d --name "$NAME" -p "${HOST_PORT}:${CONTAINER_PORT}" "$IMAGE" >/dev/null
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
