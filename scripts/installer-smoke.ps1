[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PayloadDirectory,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [Parameter(Mandatory=$true)][string]$Version,
    [Parameter(Mandatory=$true)][object]$Tools
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'installer-tools.ps1')
$identifier = [guid]::NewGuid().ToString('N')
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
$testRoot = Join-Path $outputRoot ('installer-smoke-' + $identifier)
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
$appDirectory = Join-Path $testRoot 'application'
$profileDirectory = Join-Path $testRoot 'profile'
New-Item -ItemType Directory -Path $profileDirectory | Out-Null
$sentinel = Join-Path $profileDirectory 'keep-settings.json'
[IO.File]::WriteAllText($sentinel, '{"commander":"synthetic-installer","colonies":["A","B"],"keep":true}', [Text.UTF8Encoding]::new($false))
$sentinelHash = (Get-FileHash -LiteralPath $sentinel -Algorithm SHA256).Hash
$liveProfile = Join-Path $env:LOCALAPPDATA 'ED Copilot'
function Read-ProfileDigests {
    $result = @{}
    if (Test-Path -LiteralPath $liveProfile) {
        Get-ChildItem -LiteralPath $liveProfile -File -Recurse | ForEach-Object { $result[$_.FullName] = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash }
    }
    return $result
}
$originalProfile = Read-ProfileDigests
$scenarios = [Collections.Generic.List[object]]::new()
function Confirm-Scenario {
    param([string]$Name, [bool]$Passed)
    $scenarios.Add([pscustomobject]@{name=$Name;ok=$Passed})
    if (-not $Passed) { throw "Installer smoke failed: $Name" }
}
function Invoke-InstallerProcess {
    param([string]$File, [string[]]$Arguments, [int[]]$AllowedExitCodes=@(0), [string]$Name)
    $process = Start-Process -FilePath $File -ArgumentList $Arguments -WindowStyle Hidden -PassThru
    if (-not $process.WaitForExit(60000)) { Stop-Process -Id $process.Id; throw "Installer smoke timed out: $Name" }
    if ($AllowedExitCodes -notcontains $process.ExitCode) { throw "Installer smoke $Name returned $($process.ExitCode)." }
    return $process.ExitCode
}
$testSetup = New-EDCopilotSetup -PayloadDirectory $PayloadDirectory -OutputDirectory $testRoot -Version $Version -Tools $Tools -SmokeIdentifier $identifier
$missingSetup = New-EDCopilotSetup -PayloadDirectory $PayloadDirectory -OutputDirectory $testRoot -Version $Version -Tools $Tools -SmokeIdentifier $identifier -SimulateMissingRuntime
$common = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', '/LANG=russian', ('/DIR="' + $appDirectory + '"'))
$registryPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\EDCopilot.InstallerSmoke.' + $identifier + '_is1'
Confirm-Scenario -Name 'Disposable registration starts absent' -Passed (-not (Test-Path -LiteralPath $registryPath))
$code = Invoke-InstallerProcess -File $missingSetup -Arguments ($common + @('/NOICONS', ('/LOG="' + (Join-Path $testRoot 'missing-runtime.log') + '"'))) -AllowedExitCodes @(1, 2, 7, 8) -Name 'missing runtime'
Confirm-Scenario -Name 'Missing runtime refuses silent setup without installing components' -Passed ($code -ne 0 -and -not (Test-Path -LiteralPath (Join-Path $appDirectory 'ED Copilot.exe')) -and -not (Test-Path -LiteralPath $registryPath))
$code = Invoke-InstallerProcess -File $missingSetup -Arguments ($common + @('/INSTALLWEBVIEW2', '/NOICONS', ('/LOG="' + (Join-Path $testRoot 'missing-runtime-opt-in.log') + '"'))) -AllowedExitCodes @(1, 2, 7, 8) -Name 'isolated runtime opt-in guard'
Confirm-Scenario -Name 'Isolated setup refuses component installation even with explicit runtime opt-in' -Passed ($code -ne 0 -and -not (Test-Path -LiteralPath (Join-Path $appDirectory 'ED Copilot.exe')) -and -not (Test-Path -LiteralPath $registryPath))
$mutex = [Threading.Mutex]::new($false, ('Local\EDCopilot.InstallerSmoke.' + $identifier))
try {
    $code = Invoke-InstallerProcess -File $testSetup -Arguments ($common + @('/NOICONS', ('/LOG="' + (Join-Path $testRoot 'running-app.log') + '"'))) -AllowedExitCodes @(1, 2, 7, 8) -Name 'running application guard'
    Confirm-Scenario -Name 'Running application mutex prevents replacement' -Passed ($code -ne 0 -and -not (Test-Path -LiteralPath (Join-Path $appDirectory 'ED Copilot.exe')))
}
finally { $mutex.Dispose() }
$null = Invoke-InstallerProcess -File $testSetup -Arguments ($common + @(('/LOG="' + (Join-Path $testRoot 'install.log') + '"'))) -Name 'install'
$installed = Join-Path $appDirectory 'ED Copilot.exe'
Confirm-Scenario -Name 'Executable installs into chosen directory' -Passed ((Test-Path -LiteralPath $installed) -and (Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash -eq (Get-FileHash -LiteralPath (Join-Path $PayloadDirectory 'ED Copilot.exe') -Algorithm SHA256).Hash)
$registration = Get-ItemProperty -LiteralPath $registryPath
Confirm-Scenario -Name 'Per-user uninstall registration matches isolated application' -Passed ($registration.DisplayName -eq 'ED Copilot' -and $registration.DisplayVersion -eq $Version -and [string]::Equals($registration.InstallLocation.TrimEnd('\'), $appDirectory.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase))
$shell = New-Object -ComObject WScript.Shell
Confirm-Scenario -Name 'Desktop shortcut is optional and off by default' -Passed (-not (Test-Path -LiteralPath (Join-Path $appDirectory 'smoke-desktop/ED Copilot.lnk')))
foreach ($name in @('smoke-start-menu')) {
    $path = Join-Path $appDirectory ($name + '/ED Copilot.lnk')
    $shortcut = $shell.CreateShortcut($path)
    Confirm-Scenario -Name ("Valid isolated shortcut: " + $name) -Passed ((Test-Path -LiteralPath $path) -and [string]::Equals($shortcut.TargetPath, $installed, [StringComparison]::OrdinalIgnoreCase) -and [string]::Equals($shortcut.WorkingDirectory, $appDirectory, [StringComparison]::OrdinalIgnoreCase))
}
$note = Join-Path $appDirectory 'user-note.txt'
[IO.File]::WriteAllText($note, 'User-created files must survive uninstall.', [Text.UTF8Encoding]::new($false))
$reportPath = Join-Path $testRoot 'installed-headless.json'
$null = Invoke-InstallerProcess -File $installed -Arguments @('--smoke-headless', '--smoke-report', ('"' + $reportPath + '"')) -Name 'installed application'
$model = Get-Content -LiteralPath $reportPath -Raw | ConvertFrom-Json
Confirm-Scenario -Name 'Installed executable starts with isolated network-disabled data' -Passed ($model.ok -and $model.networkDisabled -and $model.colonies -eq 2 -and $model.version -eq $Version)
$null = Invoke-InstallerProcess -File $testSetup -Arguments ($common + @('/TASKS="desktopicon"', ('/LOG="' + (Join-Path $testRoot 'upgrade.log') + '"'))) -Name 'upgrade'
Confirm-Scenario -Name 'Upgrade preserves profile and unknown files' -Passed ((Get-FileHash -LiteralPath $sentinel -Algorithm SHA256).Hash -eq $sentinelHash -and (Test-Path -LiteralPath $note))
$desktopLink = Join-Path $appDirectory 'smoke-desktop/ED Copilot.lnk'
$desktopShortcut = $shell.CreateShortcut($desktopLink)
Confirm-Scenario -Name 'Opting in creates a valid isolated desktop shortcut' -Passed ((Test-Path -LiteralPath $desktopLink) -and [string]::Equals($desktopShortcut.TargetPath, $installed, [StringComparison]::OrdinalIgnoreCase))
$uninstaller = Join-Path $appDirectory 'uninstall/unins000.exe'
Confirm-Scenario -Name 'Uninstaller is available' -Passed (Test-Path -LiteralPath $uninstaller)
$mutex = [Threading.Mutex]::new($false, ('Local\EDCopilot.InstallerSmoke.' + $identifier))
try {
    $code = Invoke-InstallerProcess -File $uninstaller -Arguments @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', ('/LOG="' + (Join-Path $testRoot 'running-uninstall.log') + '"')) -AllowedExitCodes @(1, 2, 7, 8) -Name 'running application uninstall guard'
    Confirm-Scenario -Name 'Running application mutex prevents uninstall' -Passed ($code -ne 0 -and (Test-Path -LiteralPath $installed) -and (Test-Path -LiteralPath $registryPath))
}
finally { $mutex.Dispose() }
$null = Invoke-InstallerProcess -File $uninstaller -Arguments @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', ('/LOG="' + (Join-Path $testRoot 'uninstall.log') + '"')) -Name 'uninstall'
for ($i=0; $i -lt 30 -and (Test-Path -LiteralPath $installed); $i++) { Start-Sleep -Milliseconds 100 }
Confirm-Scenario -Name 'Uninstall removes managed executable and registration' -Passed (-not (Test-Path -LiteralPath $installed) -and -not (Test-Path -LiteralPath $registryPath))
Confirm-Scenario -Name 'Uninstall removes managed shortcuts' -Passed (-not (Test-Path -LiteralPath $desktopLink) -and -not (Test-Path -LiteralPath (Join-Path $appDirectory 'smoke-start-menu/ED Copilot.lnk')))
Confirm-Scenario -Name 'Uninstall preserves profile and user-created app files' -Passed ((Get-FileHash -LiteralPath $sentinel -Algorithm SHA256).Hash -eq $sentinelHash -and (Test-Path -LiteralPath $note))
$after = Read-ProfileDigests
$unchanged = $after.Count -eq $originalProfile.Count
foreach ($file in $originalProfile.Keys) { if (-not $after.ContainsKey($file) -or $after[$file] -ne $originalProfile[$file]) { $unchanged = $false } }
Confirm-Scenario -Name 'Original player profile remains byte-identical' -Passed $unchanged
$report = [ordered]@{ok=$true;version=$Version;isolated=$true;profileUnchanged=$unchanged;networkDisabled=$true;scenarios=$scenarios;windowsComponentsInstalled=$false}
$json = $report | ConvertTo-Json -Depth 8
[IO.File]::WriteAllText((Join-Path $outputRoot 'smoke-installer.json'), $json, [Text.UTF8Encoding]::new($false))
Write-Output "Installer smoke: $($scenarios.Count) scenarios passed."
