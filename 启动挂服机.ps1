$ErrorActionPreference = 'Stop'
$taskUrl = 'http://127.0.0.1:18323/'
try {
    $taskStatus = Invoke-RestMethod -Uri ($taskUrl + 'api/status') -TimeoutSec 2
    Start-Process $taskUrl
    Write-Output ('挂服机已运行，房间号：' + $taskStatus.room_id)
    exit
} catch { }
$taskBds = @(Get-NetUDPEndpoint -LocalPort 19132 -ErrorAction SilentlyContinue)
if ($taskBds.Count -eq 0) {
    $taskBdsScript = Join-Path $PSScriptRoot 'runtime/start-bedrock.ps1'
    if (Test-Path -LiteralPath $taskBdsScript) { & $taskBdsScript } else { Write-Output '请先运行自己的 Bedrock 服务器，或在控制台设置已有服务器地址。' }
}
$taskExe = @('LobbyKeeper.exe', 'LobbyKeeper-session-guard.exe') | ForEach-Object { Join-Path $PSScriptRoot $_ } | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if (-not $taskExe) { throw '未找到程序，请先运行构建.ps1 或下载 Windows 发布版。' }
$taskData = Join-Path $PSScriptRoot 'current'
Start-Process -FilePath $taskExe -ArgumentList @('-data-dir', ('"' + $taskData + '"'), '-web-port', '18323') -WorkingDirectory $PSScriptRoot -WindowStyle Hidden
Write-Output '挂服机已启动，控制台：http://127.0.0.1:18323/'
