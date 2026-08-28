#!/usr/bin/env bash
# Build (if needed) and run the local Petstore 3 OpenAPI server in Docker.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
IMAGE="${PETSTORE_IMAGE:-context-mesh-petstore3:local}"
NAME="${PETSTORE_CONTAINER:-petstore-openapi-server}"
HOST_PORT="${PETSTORE_PORT:-8090}"
UPSTREAM="${PETSTORE_UPSTREAM:-swaggerapi/petstore3:latest}"
DEFAULT_SLF4J_URL="https://repo1.maven.org/maven2/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar"
DEFAULT_SLF4J_SHA256="2f39bed943d624dfa8f4102d0571283a10870b6aa36f197a8a506f147010c10f"
SLF4J="${PETSTORE_SLF4J:-$DEFAULT_SLF4J_URL}"
SLF4J_SKIP=0
if [[ "${PETSTORE_SLF4J:-}" == "skip" ]]; then
  SLF4J_SKIP=1
fi
PLATFORM="${PETSTORE_PLATFORM:-linux/amd64}"
PULL_TIMEOUT="${PETSTORE_PULL_TIMEOUT:-120}"
CONTAINER_PORT=8080
BASE_URL="http://localhost:${HOST_PORT}/api/v3"

usage() {
  cat <<EOF
usage: $(basename "$0") [--rebuild] [--upstream IMAGE] [--slf4j URL|PATH] [--no-slf4j]

  --rebuild           remove the local image/container and rebuild from the Dockerfile
  --upstream IMAGE    base image (default swaggerapi/petstore3:latest).
                      Overrides PETSTORE_UPSTREAM. Use --rebuild to apply a new base
                      when context-mesh-petstore3:local already exists.
  --slf4j URL|PATH    SLF4J 1.7 binding jar (http(s) URL or local file).
                      Default is slf4j-simple from Maven Central.
                      Any 1.7 impl works (slf4j-simple, slf4j-log4j12, …);
                      the source filename is not assumed. Overrides PETSTORE_SLF4J.
                      Use --rebuild to apply a new jar when the local image exists.
  --no-slf4j          do not download or install an SLF4J binding (Jetty stays noisy).
                      Overrides PETSTORE_SLF4J=skip.

Runs Petstore 3 in the **foreground**. Ctrl+C (or SIGTERM) stops the container
and removes it (--rm), which deletes in-memory users/orders.

Env:
  PETSTORE_IMAGE         local tag (default context-mesh-petstore3:local)
  PETSTORE_CONTAINER     container name (default petstore-openapi-server)
  PETSTORE_PORT          host port mapped to 8080 (default 8090)
  PETSTORE_UPSTREAM      base image to pull/build from (default swaggerapi/petstore3:latest)
  PETSTORE_SLF4J         SLF4J 1.7 binding URL, local path, or skip (default Maven Central slf4j-simple)
  PETSTORE_PLATFORM      pull/run platform (default linux/amd64)
  PETSTORE_PULL_TIMEOUT  seconds to wait for docker pull (default 120)
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

need_value() {
  local flag=$1 val=${2:-}
  if [[ -z "$val" ]]; then
    echo "${flag} requires a value" >&2
    usage >&2
    exit 2
  fi
}

REBUILD=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --rebuild)
      REBUILD=1
      shift
      ;;
    --no-slf4j)
      SLF4J_SKIP=1
      shift
      ;;
    --upstream)
      need_value "$1" "${2:-}"
      UPSTREAM="$2"
      shift 2
      ;;
    --upstream=*)
      UPSTREAM="${1#*=}"
      need_value "--upstream" "$UPSTREAM"
      shift
      ;;
    --slf4j)
      need_value "$1" "${2:-}"
      SLF4J="$2"
      if [[ "$SLF4J" == skip ]]; then
        SLF4J_SKIP=1
      else
        SLF4J_SKIP=0
      fi
      shift 2
      ;;
    --slf4j=*)
      SLF4J="${1#*=}"
      need_value "--slf4j" "$SLF4J"
      if [[ "$SLF4J" == skip ]]; then
        SLF4J_SKIP=1
      else
        SLF4J_SKIP=0
      fi
      shift
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

is_http_url() {
  case "$1" in
    http://*|https://*) return 0 ;;
    *) return 1 ;;
  esac
}

resolve_local_jar() {
  local src=$1
  if [[ "$src" == file://* ]]; then
    src="${src#file://}"
  fi
  if [[ ! -f "$src" ]]; then
    echo "slf4j jar not found: $src" >&2
    exit 1
  fi
  local dir
  dir="$(cd "$(dirname "$src")" && pwd)"
  echo "$dir/$(basename "$src")"
}

build_image() {
  local mode url sha jar stage
  sha=""
  url=""
  jar=""
  if [[ "$SLF4J_SKIP" -eq 1 ]]; then
    mode=skip
  elif is_http_url "$SLF4J"; then
    mode=url
    url="$SLF4J"
    if [[ "$url" == "$DEFAULT_SLF4J_URL" ]]; then
      sha="$DEFAULT_SLF4J_SHA256"
    fi
  else
    mode=file
    jar="$(resolve_local_jar "$SLF4J")"
  fi

  stage=$(mktemp -d)
  mkdir -p "$stage/slf4j.bundle"
  : > "$stage/slf4j.bundle/.keep"
  cp "$ROOT/Dockerfile" "$ROOT/jetty-context.xml" "$ROOT/jetty-context-noslf4j.xml" "$stage/"
  if [[ -n "$jar" ]]; then
    cp "$jar" "$stage/slf4j.bundle/slf4j-impl.jar"
  fi

  echo "building ${IMAGE} (--platform ${PLATFORM}, base ${UPSTREAM}, slf4j ${mode}${url:+ ${url}}${jar:+ ${jar}})..."
  if ! docker build --platform "$PLATFORM" \
    --build-arg PETSTORE_UPSTREAM="$UPSTREAM" \
    --build-arg SLF4J_MODE="$mode" \
    --build-arg SLF4J_URL="$url" \
    --build-arg SLF4J_SHA256="$sha" \
    -t "$IMAGE" "$stage"; then
    rm -rf "$stage"
    return 1
  fi
  rm -rf "$stage"
}

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
else
  if [[ "$REBUILD" -eq 1 ]] || ! image_exists "$UPSTREAM"; then
    pull_upstream
  else
    echo "using local ${UPSTREAM} as build base (skipping pull)"
  fi
  build_image
fi

if docker ps -a --format '{{.Names}}' | grep -qx "$NAME"; then
  echo "removing leftover container ${NAME}"
  docker rm -f "$NAME" >/dev/null 2>&1 || true
fi

echo "starting container ${NAME} on localhost:${HOST_PORT} (foreground, --rm)"
docker run --rm --name "$NAME" --platform "$PLATFORM" \
  -p "${HOST_PORT}:${CONTAINER_PORT}" "$IMAGE" &
dockpid=$!

cleanup() {
  trap - EXIT INT TERM
  if kill -0 "$dockpid" 2>/dev/null; then
    kill "$dockpid" 2>/dev/null || true
    wait "$dockpid" 2>/dev/null || true
  fi
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "waiting for ${BASE_URL}/openapi.json ..."
ready=0
for _ in $(seq 1 60); do
  if ! kill -0 "$dockpid" 2>/dev/null; then
    echo "container exited before ready" >&2
    wait "$dockpid" || true
    exit 1
  fi
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
echo "Ctrl+C stops the container and deletes its data"

wait "$dockpid"
status=$?
trap - EXIT INT TERM
exit "$status"
