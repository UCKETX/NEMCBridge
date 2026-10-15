$ErrorActionPreference = 'Stop'
$taskOrigin = 'http://127.0.0.1:18323'
$taskLoginRequired = $false
try {
    $null = Invoke-RestMethod -Uri ($taskOrigin + '/api/status') -TimeoutSec 2
} catch {
    if ($_.Exception.Response -and [int]$_.Exception.Response.StatusCode -eq 401) {
        $taskLoginRequired = $true
    } else { throw }
}
if ($taskLoginRequired) {
    $taskPassword = $env:NEMCBRIDGE_ADMIN_PASSWORD
    if ([string]::IsNullOrEmpty($taskPassword)) {
        $taskSecure = Read-Host '请输入 NEMCBridge 管理密码' -AsSecureString
        $taskPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($taskSecure)
        try { $taskPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($taskPointer) }
        finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($taskPointer) }
    }
    try {
        $taskBody = @{ username = 'admin'; password = $taskPassword } | ConvertTo-Json -Compress
        Invoke-RestMethod -Uri ($taskOrigin + '/api/login') -Method Post -Headers @{Origin=$taskOrigin} -ContentType 'application/json; charset=utf-8' -Body ([Text.Encoding]::UTF8.GetBytes($taskBody)) -SessionVariable taskWebSession | Out-Null
    } finally { $taskPassword = $null; $taskBody = $null }
    Invoke-RestMethod -Uri ($taskOrigin + '/api/exit') -Method Post -Headers @{Origin=$taskOrigin} -ContentType 'application/json' -Body '{}' -WebSession $taskWebSession | Out-Null
} else {
    Invoke-RestMethod -Uri ($taskOrigin + '/api/exit') -Method Post -Headers @{Origin=$taskOrigin} -ContentType 'application/json' -Body '{}' | Out-Null
}
Write-Output '已请求正常关房并退出挂服机。Bedrock 服务器继续运行。'
