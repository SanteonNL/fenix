$hixTestActive = (Get-Content "config/config.yaml") |
    Where-Object { $_ -match '^\s+hix-test:' -and $_ -notmatch '^\s*#' }
if (-not $hixTestActive) {
    Write-Host 'hix-test not in config sources — skipping Docker startup.'
    exit 0
}

function Get-ContainerHealth {
    $json = docker inspect hix-test-sqlserver 2>$null | ConvertFrom-Json
    return $json.State.Health.Status
}

if ((Get-ContainerHealth) -eq 'healthy') {
    Write-Host 'SQL Server already healthy.'
    exit 0
}

$linuxPipe = '\\.\pipe\dockerDesktopLinuxEngine'
if (-not (Test-Path $linuxPipe)) {
    Write-Host 'Starting Docker Desktop...'
    Start-Process 'C:\Program Files\Docker\Docker\Docker Desktop.exe'
    Write-Host 'Waiting for Docker Linux engine...'
    while (-not (Test-Path $linuxPipe)) { Start-Sleep 2 }
    Write-Host 'Docker Linux engine is ready.'
}

Write-Host 'Starting hix-test container...'
docker-compose -f test/hix-test/docker-compose.yml up -d

Write-Host 'Waiting for hix-test-sqlserver to become healthy...'
$timeout = 120
$elapsed = 0
do {
    Start-Sleep 3
    $elapsed += 3
    $health = Get-ContainerHealth
    Write-Host "  [$elapsed s] status: $health"
} while ($health -ne 'healthy' -and $elapsed -lt $timeout)

if ($health -eq 'healthy') {
    Write-Host "SQL Server is healthy (${elapsed}s)."
} else {
    Write-Host "Timed out after ${timeout}s. Last status: $health"
    exit 1
}
