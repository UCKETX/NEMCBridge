$ErrorActionPreference = 'Stop'
$taskUrl = 'http://127.0.0.1:18323/'
$taskRoot = Split-Path -Parent $PSScriptRoot
try {
    $null = Invoke-WebRequest -Uri ($taskUrl + 'login') -UseBasicParsing -TimeoutSec 2
    Start-Process $taskUrl
    Write-Output 'NEMCBridge 已运行，请登录管理页面查看房间。'
    exit
} catch { }
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
$taskExe = Join-Path $taskRoot 'build/NEMCBridge.exe'
if (-not (Test-Path -LiteralPath $taskExe)) { $taskExe = $null }
if (-not $taskExe) { throw '未找到程序，请先运行 scripts/build.ps1 或下载 Windows 发布版。' }
$taskData = Join-Path $taskRoot 'data/dev'
& $taskExe -data-dir $taskData -web-port 18323
