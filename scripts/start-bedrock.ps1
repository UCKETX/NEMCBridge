$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskServer = Join-Path $taskRoot 'runtime/bedrock-1.21.124.2'
$taskExe = Join-Path $taskServer 'bedrock_server.exe'
if (-not (Test-Path -LiteralPath $taskExe)) {
    throw 'Run scripts/download-bedrock.py to install official Bedrock 1.21.124.2 first.'
}
$taskPorts = @(Get-NetUDPEndpoint -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -in 19132,19133 })
if ($taskPorts.Count -gt 0) {
    throw "UDP 19132 or 19133 is already occupied; refusing to start another server."
}
$taskProperties = Join-Path $taskServer 'server.properties'
$taskOriginal = Join-Path $taskServer 'server.properties.original'
if (-not (Test-Path -LiteralPath $taskOriginal)) {
    Copy-Item -LiteralPath $taskProperties -Destination $taskOriginal
}
$taskSettings = [ordered]@{
    'server-name' = 'NEMCBridge 1.21.124 Test'
    'gamemode' = 'creative'
    'difficulty' = 'peaceful'
    'allow-cheats' = 'true'
    'max-players' = '10'
    'online-mode' = 'false'
    'server-port' = '19132'
    'server-portv6' = '19133'
    'enable-lan-visibility' = 'false'
    'view-distance' = '4'
    'tick-distance' = '4'
    'max-threads' = '2'
    'player-idle-timeout' = '0'
    'level-name' = 'LobbyKeeper-Test-1.21.124' # Preserve existing worlds across the rename.
    'level-seed' = '20261015'
}
$taskContent = [IO.File]::ReadAllText($taskProperties)
foreach ($taskEntry in $taskSettings.GetEnumerator()) {
    $taskPattern = '(?m)^' + [regex]::Escape($taskEntry.Key) + '=.*$'
    $taskContent = [regex]::Replace($taskContent, $taskPattern, ($taskEntry.Key + '=' + $taskEntry.Value))
}
[IO.File]::WriteAllText($taskProperties, $taskContent, [Text.UTF8Encoding]::new($false))
$taskTimestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$taskLog = Join-Path $taskServer "bedrock-$taskTimestamp.stdout.log"
$taskErrorLog = Join-Path $taskServer "bedrock-$taskTimestamp.stderr.log"
$taskProcess = Start-Process -FilePath $taskExe -WorkingDirectory $taskServer -WindowStyle Hidden -PassThru -RedirectStandardOutput $taskLog -RedirectStandardError $taskErrorLog
$taskProcess.Id | Set-Content -LiteralPath (Join-Path $taskServer 'bedrock.pid')
$taskState = [ordered]@{
    pid = $taskProcess.Id
    version = '1.21.124.2'
    connect_address = '127.0.0.1:19132'
    bind_note = 'Official BDS has no documented server-ip option and binds available interfaces.'
    started_at = (Get-Date).ToString('o')
    stdout_log = $taskLog
    stderr_log = $taskErrorLog
}
$taskState | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $taskServer 'bedrock-process.json') -Encoding utf8
Write-Output ($taskState | ConvertTo-Json)
