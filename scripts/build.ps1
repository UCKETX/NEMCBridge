$ErrorActionPreference = 'Stop'
$env:GOTOOLCHAIN = 'go1.26.9'
$taskRoot = Split-Path -Parent $PSScriptRoot
Push-Location $taskRoot
try {
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw '自动测试失败' }
    go test github.com/Happy2018new/nemc-tan-lobby-solver/core/sctp -run 'TestChecksumWireCompatibility|TestPacketUnmarshal|TestPacketMarshal'
    if ($LASTEXITCODE -ne 0) { throw 'SCTP 校验格式测试失败' }
    go build -trimpath -o LobbyKeeper.exe .
    if ($LASTEXITCODE -ne 0) { throw '构建失败' }
    Write-Output '构建成功：LobbyKeeper.exe。启动脚本会使用此程序；若文件正在运行，请先正常关闭挂服机再构建。'
} finally { Pop-Location }
