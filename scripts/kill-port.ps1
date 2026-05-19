param([int]$Port = 8080)

$match = netstat -ano | Select-String ":$Port\s.*LISTENING"
if ($match) {
    $procId = $match.ToString().Trim().Split()[-1]
    Stop-Process -Id $procId -Force
    Write-Host "Killed PID $procId on port $Port"
} else {
    Write-Host "Nothing listening on port $Port"
}
