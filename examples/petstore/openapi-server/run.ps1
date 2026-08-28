# Build (if needed) and run the local Petstore 3 OpenAPI server in Docker.
param(
    [switch]$Rebuild,
    [switch]$Help,
    [string]$Upstream = ""
)

$ErrorActionPreference = "Stop"

if ($Help) {
    Write-Host @"
usage: ./run.ps1 [-Rebuild] [-Upstream IMAGE]

  -Rebuild    remove the local image/container and rebuild from the Dockerfile
  -Upstream   base image (default swaggerapi/petstore3:latest).
              Overrides PETSTORE_UPSTREAM. Use -Rebuild to apply a new base
              when context-mesh-petstore3:local already exists.

Runs Petstore 3 in the foreground. Ctrl+C stops the container and removes it
(--rm), which deletes in-memory users/orders.

Env:
  PETSTORE_IMAGE         local tag (default context-mesh-petstore3:local)
  PETSTORE_CONTAINER     container name (default petstore-openapi-server)
  PETSTORE_PORT          host port mapped to 8080 (default 8090)
  PETSTORE_UPSTREAM      base image to pull/build from (default swaggerapi/petstore3:latest)
  PETSTORE_PLATFORM      pull/run platform (default linux/amd64)
  PETSTORE_PULL_TIMEOUT  seconds to wait for docker pull (default 120)
"@
    exit 0
}

$Image = if ($env:PETSTORE_IMAGE) { $env:PETSTORE_IMAGE } else { "context-mesh-petstore3:local" }
$Name = if ($env:PETSTORE_CONTAINER) { $env:PETSTORE_CONTAINER } else { "petstore-openapi-server" }
$HostPort = if ($env:PETSTORE_PORT) { $env:PETSTORE_PORT } else { "8090" }
if (-not $Upstream) {
    $Upstream = if ($env:PETSTORE_UPSTREAM) { $env:PETSTORE_UPSTREAM } else { "swaggerapi/petstore3:latest" }
}
$Platform = if ($env:PETSTORE_PLATFORM) { $env:PETSTORE_PLATFORM } else { "linux/amd64" }
$PullTimeout = if ($env:PETSTORE_PULL_TIMEOUT) { [int]$env:PETSTORE_PULL_TIMEOUT } else { 120 }
$ContainerPort = 8080
$BaseUrl = "http://localhost:${HostPort}/api/v3"

function Write-RegistryHelp {
    Write-Host @"

docker could not pull $Upstream (often a Docker Desktop + VPN registry hang).

Host DNS can work while the Docker VM cannot. Try:
  1. Disconnect VPN
  2. Restart Docker Desktop
  3. Re-run this script

Or skip Docker and use the hosted API:
  go run ./examples/petstore/async-order-server -petstore hosted
  go run ./examples/petstore/mcp-server -petstore hosted
"@
}

function Test-Docker {
    try {
        docker version | Out-Null
    } catch {
        Write-Error "docker is required (Docker Desktop or engine on PATH)"
        exit 1
    }
}

function Test-LocalImage {
    param([string]$Tag)
    docker image inspect $Tag 1>$null 2>$null
    return ($LASTEXITCODE -eq 0)
}

function Get-ContainerState {
    param([string]$ContainerName)
    $running = docker ps --format "{{.Names}}" | Where-Object { $_ -eq $ContainerName }
    if ($running) { return "running" }
    $any = docker ps -a --format "{{.Names}}" | Where-Object { $_ -eq $ContainerName }
    if ($any) { return "stopped" }
    return "missing"
}

function Invoke-PullUpstream {
    Write-Host "pulling $Upstream (--platform $Platform, timeout ${PullTimeout}s)..."
    $job = Start-Job -ScriptBlock {
        param($Image, $Plat)
        docker pull --platform $Plat $Image
        if ($LASTEXITCODE -ne 0) { throw "docker pull failed: $LASTEXITCODE" }
    } -ArgumentList $Upstream, $Platform
    if (-not (Wait-Job $job -Timeout $PullTimeout)) {
        Stop-Job $job -ErrorAction SilentlyContinue
        Remove-Job $job -Force -ErrorAction SilentlyContinue
        Write-Error "timed out after ${PullTimeout}s pulling $Upstream"
        Write-RegistryHelp
        exit 1
    }
    Receive-Job $job
    if ($job.State -ne "Completed") {
        Remove-Job $job -Force -ErrorAction SilentlyContinue
        Write-RegistryHelp
        exit 1
    }
    Remove-Job $job -Force -ErrorAction SilentlyContinue
}

Test-Docker

if ($Rebuild) {
    docker rm -f $Name 2>$null | Out-Null
    docker image rm -f $Image 2>$null | Out-Null
}

if (Test-LocalImage $Image) {
    Write-Host "using existing image $Image"
} else {
    if ($Rebuild -or -not (Test-LocalImage $Upstream)) {
        Invoke-PullUpstream
    } else {
        Write-Host "using local $Upstream as build base (skipping pull)"
    }
    Write-Host "building $Image (--platform $Platform, base $Upstream)..."
    docker build --platform $Platform --build-arg "PETSTORE_UPSTREAM=$Upstream" -t $Image $PSScriptRoot
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

switch (Get-ContainerState $Name) {
    "running" {
        Write-Host "removing leftover container $Name"
        docker rm -f $Name 2>$null | Out-Null
    }
    "stopped" {
        Write-Host "removing leftover container $Name"
        docker rm -f $Name 2>$null | Out-Null
    }
}

Write-Host "starting container $Name on localhost:${HostPort} (foreground, --rm)"
$run = Start-Process -FilePath "docker" -ArgumentList @(
    "run", "--rm", "--name", $Name, "--platform", $Platform,
    "-p", "${HostPort}:${ContainerPort}", $Image
) -NoNewWindow -PassThru

try {
    Write-Host "waiting for $BaseUrl/openapi.json ..."
    $ready = $false
    for ($i = 0; $i -lt 60; $i++) {
        if ($run.HasExited) {
            Write-Error "container exited before ready"
            exit 1
        }
        try {
            Invoke-WebRequest -Uri "$BaseUrl/openapi.json" -UseBasicParsing -TimeoutSec 2 | Out-Null
            $ready = $true
            break
        } catch {
            try {
                Invoke-WebRequest -Uri "$BaseUrl/pet/findByStatus?status=available" -UseBasicParsing -TimeoutSec 2 | Out-Null
                $ready = $true
                break
            } catch {
                Start-Sleep -Seconds 1
            }
        }
    }
    if (-not $ready) {
        Write-Error "timed out waiting for Petstore at $BaseUrl"
        docker logs $Name --tail 40
        exit 1
    }

    Write-Host "Petstore 3 OpenAPI: $BaseUrl"
    Write-Host "Swagger UI:         http://localhost:${HostPort}/"
    Write-Host "Ctrl+C stops the container and deletes its data"
    Wait-Process -Id $run.Id
    exit $run.ExitCode
} finally {
    if (-not $run.HasExited) {
        Stop-Process -Id $run.Id -ErrorAction SilentlyContinue
        try { Wait-Process -Id $run.Id -Timeout 5 } catch { }
    }
    docker rm -f $Name 2>$null | Out-Null
}
