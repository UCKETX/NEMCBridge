$ErrorActionPreference = 'Stop'
$taskUrl = 'http://127.0.0.1:18323/'
$taskRoot = Split-Path -Parent $PSScriptRoot
try {
    $taskStatus = Invoke-RestMethod -Uri ($taskUrl + 'api/status') -TimeoutSec 2
    Start-Process $taskUrl
    Write-Output ('挂服机已运行，房间号：' + $taskStatus.room_id)
    exit
} catch { }
$taskBds = @(Get-NetUDPEndpoint -LocalPort 19132 -ErrorAction SilentlyContinue)
if ($taskBds.Count -eq 0) {
    $taskBdsScript = Join-Path $PSScriptRoot 'start-bedrock.ps1'
    if (Test-Path -LiteralPath $taskBdsScript) { & $taskBdsScript } else { Write-Output '请先运行自己的 Bedrock 服务器，或在控制台设置已有服务器地址。' }
}
$taskExe = @('NEMCBridge.exe', 'LobbyKeeper.exe') | ForEach-Object { Join-Path $taskRoot $_ } | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if (-not $taskExe) { throw '未找到程序，请先运行 scripts/build.ps1 或下载 Windows 发布版。' }
$taskData = Join-Path $taskRoot 'current'
Start-Process -FilePath $taskExe -ArgumentList @('-data-dir', ('"' + $taskData + '"'), '-web-port', '18323') -WorkingDirectory $taskRoot -WindowStyle Hidden
Write-Output '挂服机已启动，控制台：http://127.0.0.1:18323/'
