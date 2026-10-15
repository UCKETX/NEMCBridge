$ErrorActionPreference = 'Stop'
$taskOrigin = 'http://127.0.0.1:18323'
Invoke-RestMethod -Uri ($taskOrigin + '/api/exit') -Method Post -Headers @{Origin=$taskOrigin} -ContentType 'application/json' -Body '{}' | Out-Null
Write-Output '已请求正常关房并退出挂服机。Bedrock 服务器继续运行。'
