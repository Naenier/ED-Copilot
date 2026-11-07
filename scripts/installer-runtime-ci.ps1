$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:CI -ne 'true' -or $env:GITHUB_ACTIONS -ne 'true') { throw 'Automatic runtime provisioning is restricted to the disposable GitHub Actions runner.' }
. (Join-Path $PSScriptRoot 'installer-tools.ps1')
function Test-WebView2Runtime {
    $key = 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
    foreach ($hive in @([Microsoft.Win32.RegistryHive]::CurrentUser, [Microsoft.Win32.RegistryHive]::LocalMachine)) {
        $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey($hive, [Microsoft.Win32.RegistryView]::Registry32)
        try {
            $runtime = $base.OpenSubKey($key)
            if ($runtime) {
                try {
                    $text = [string]$runtime.GetValue('pv', '')
                    $parsed = $null
                    if ([version]::TryParse($text, [ref]$parsed) -and $parsed -gt [version]'0.0.0.0') { return $true }
                }
                finally { $runtime.Dispose() }
            }
        }
        finally { $base.Dispose() }
    }
    return $false
}
if (Test-WebView2Runtime) { Write-Output 'WebView2 Runtime is already present.'; return }
$projectRoot = Split-Path $PSScriptRoot -Parent
$tools = Get-EDCopilotInstallerTools -ToolsDirectory (Join-Path $projectRoot 'dist/tools')
$process = Start-Process -FilePath $tools.Bootstrapper -ArgumentList @('/silent', '/install') -WindowStyle Hidden -PassThru
if (-not $process.WaitForExit(180000)) { Stop-Process -Id $process.Id; throw 'CI WebView2 Runtime provisioning timed out.' }
if ($process.ExitCode -notin @(0,3010) -or -not (Test-WebView2Runtime)) { throw "CI WebView2 Runtime provisioning failed ($($process.ExitCode))." }
Write-Output 'Verified Microsoft WebView2 Runtime is ready on the disposable CI runner.'
