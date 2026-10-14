$ErrorActionPreference = 'Stop'
$env:GOTOOLCHAIN = 'go1.26.9'
Push-Location $PSScriptRoot
try {
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw '自动测试失败' }
    go test github.com/Happy2018new/nemc-tan-lobby-solver/core/sctp -run 'TestChecksumWireCompatibility|TestPacketUnmarshal|TestPacketMarshal'
    if ($LASTEXITCODE -ne 0) { throw 'SCTP 校验格式测试失败' }
    go build -trimpath -o LobbyKeeper-build.exe .
    if ($LASTEXITCODE -ne 0) { throw '构建失败' }
    Write-Output '构建成功：LobbyKeeper-build.exe。请先正常关闭运行中的挂服机，再更新启动脚本使用的程序。'
} finally { Pop-Location }
