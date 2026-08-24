# Build (if needed) and run the local Petstore 3 OpenAPI server in Docker.
param(
    [switch]$Rebuild,
    [switch]$Help
)

$ErrorActionPreference = "Stop"

if ($Help) {
    Write-Host @"
usage: ./run.ps1 [-Rebuild]

  -Rebuild   remove the local image/container and build again

Env:
  PETSTORE_IMAGE      docker image tag (default context-mesh-petstore3:local)
  PETSTORE_CONTAINER  container name (default petstore-openapi-server)
  PETSTORE_PORT       host port mapped to container 8080 (default 8090)
"@
    exit 0
}

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Image = if ($env:PETSTORE_IMAGE) { $env:PETSTORE_IMAGE } else { "context-mesh-petstore3:local" }
$Name = if ($env:PETSTORE_CONTAINER) { $env:PETSTORE_CONTAINER } else { "petstore-openapi-server" }
$HostPort = if ($env:PETSTORE_PORT) { $env:PETSTORE_PORT } else { "8090" }
$ContainerPort = 8080
$BaseUrl = "http://localhost:${HostPort}/api/v3"

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

Test-Docker

if ($Rebuild) {
    docker rm -f $Name 2>$null | Out-Null
    docker image rm -f $Image 2>$null | Out-Null
}

if (-not (Test-LocalImage $Image)) {
    Write-Host "image $Image not found; building (pulls swaggerapi/petstore3:unstable once)..."
    docker build -t $Image $Root
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} else {
    Write-Host "using existing image $Image"
}

switch (Get-ContainerState $Name) {
    "running" { Write-Host "container $Name already running" }
    "stopped" {
        Write-Host "starting existing container $Name"
        docker start $Name | Out-Null
    }
    default {
        Write-Host "starting container $Name on localhost:${HostPort}"
        docker run -d --name $Name -p "${HostPort}:${ContainerPort}" $Image | Out-Null
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }
}

Write-Host "waiting for $BaseUrl/openapi.json ..."
$ready = $false
for ($i = 0; $i -lt 60; $i++) {
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
Write-Host "stop: docker stop $Name"
