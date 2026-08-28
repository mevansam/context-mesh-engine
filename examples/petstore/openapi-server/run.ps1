# Build (if needed) and run the local Petstore 3 OpenAPI server in Docker.
param(
    [switch]$Rebuild,
    [switch]$Help,
    [switch]$NoSlf4j,
    [string]$Upstream = "",
    [string]$Slf4j = ""
)

$ErrorActionPreference = "Stop"

$DefaultSlf4jUrl = "https://repo1.maven.org/maven2/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar"
$DefaultSlf4jSha256 = "2f39bed943d624dfa8f4102d0571283a10870b6aa36f197a8a506f147010c10f"

if ($Help) {
    Write-Host @"
usage: ./run.ps1 [-Rebuild] [-Upstream IMAGE] [-Slf4j URL|PATH] [-NoSlf4j]

  -Rebuild     remove the local image/container and rebuild from the Dockerfile
  -Upstream    base image (default swaggerapi/petstore3:latest).
               Overrides PETSTORE_UPSTREAM. Use -Rebuild to apply a new base
               when context-mesh-petstore3:local already exists.
  -Slf4j       SLF4J 1.7 binding jar (http(s) URL or local file).
               Default is slf4j-simple from Maven Central.
               Any 1.7 impl works (slf4j-simple, slf4j-log4j12, …);
               the source filename is not assumed. Overrides PETSTORE_SLF4J.
               Use -Rebuild to apply a new jar when the local image exists.
  -NoSlf4j     do not download or install an SLF4J binding (Jetty stays noisy).
               Overrides PETSTORE_SLF4J=skip.

Runs Petstore 3 in the foreground. Ctrl+C stops the container and removes it
(--rm), which deletes in-memory users/orders.

Env:
  PETSTORE_IMAGE         local tag (default context-mesh-petstore3:local)
  PETSTORE_CONTAINER     container name (default petstore-openapi-server)
  PETSTORE_PORT          host port mapped to 8080 (default 8090)
  PETSTORE_UPSTREAM      base image to pull/build from (default swaggerapi/petstore3:latest)
  PETSTORE_SLF4J         SLF4J 1.7 binding URL, local path, or skip (default Maven Central slf4j-simple)
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
if (-not $Slf4j) {
    $Slf4j = if ($env:PETSTORE_SLF4J) { $env:PETSTORE_SLF4J } else { $DefaultSlf4jUrl }
}
if (-not $NoSlf4j -and $Slf4j -eq "skip") {
    $NoSlf4j = $true
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

function Test-HttpUrl {
    param([string]$Value)
    return $Value -match '^https?://'
}

function Resolve-LocalJar {
    param([string]$Src)
    if ($Src.StartsWith("file://")) {
        $Src = $Src.Substring("file://".Length)
    }
    if (-not (Test-Path -LiteralPath $Src -PathType Leaf)) {
        Write-Error "slf4j jar not found: $Src"
        exit 1
    }
    return (Resolve-Path -LiteralPath $Src).Path
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

function Invoke-BuildImage {
    $mode = "url"
    $url = ""
    $sha = ""
    $jar = ""
    if ($NoSlf4j) {
        $mode = "skip"
    } elseif (Test-HttpUrl $Slf4j) {
        $mode = "url"
        $url = $Slf4j
        if ($url -eq $DefaultSlf4jUrl) {
            $sha = $DefaultSlf4jSha256
        }
    } else {
        $mode = "file"
        $jar = Resolve-LocalJar $Slf4j
    }

    $stage = Join-Path ([System.IO.Path]::GetTempPath()) ("petstore-openapi-" + [guid]::NewGuid().ToString())
    New-Item -ItemType Directory -Path (Join-Path $stage "slf4j.bundle") | Out-Null
    try {
        New-Item -ItemType File -Path (Join-Path $stage "slf4j.bundle/.keep") | Out-Null
        Copy-Item (Join-Path $PSScriptRoot "Dockerfile") $stage
        Copy-Item (Join-Path $PSScriptRoot "jetty-context.xml") $stage
        Copy-Item (Join-Path $PSScriptRoot "jetty-context-noslf4j.xml") $stage
        if ($jar) {
            Copy-Item $jar (Join-Path $stage "slf4j.bundle/slf4j-impl.jar")
        }
        $extra = if ($url) { " $url" } elseif ($jar) { " $jar" } else { "" }
        Write-Host "building $Image (--platform $Platform, base $Upstream, slf4j $mode$extra)..."
        docker build --platform $Platform `
            --build-arg "PETSTORE_UPSTREAM=$Upstream" `
            --build-arg "SLF4J_MODE=$mode" `
            --build-arg "SLF4J_URL=$url" `
            --build-arg "SLF4J_SHA256=$sha" `
            -t $Image $stage
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
        Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue
    }
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
    Invoke-BuildImage
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
