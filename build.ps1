[CmdletBinding()]
param([switch]$Smoke, [switch]$Package, [switch]$Installer, [switch]$SkipTests)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'This build targets native Windows x64 controls.' }
$projectRoot = $PSScriptRoot
$savedGoEnvironment = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
Push-Location -LiteralPath $projectRoot
try {
    function Invoke-Go {
        param([string[]]$Arguments)
        & go @Arguments
        if ($LASTEXITCODE -ne 0) { throw "go command failed ($LASTEXITCODE): $($Arguments -join ' ')" }
    }
    Invoke-Go -Arguments @('version')
    Invoke-Go -Arguments @('mod', 'verify')
    if (-not $SkipTests) {
        Invoke-Go -Arguments @('test', './...', '-count=1', '-timeout=120s')
        Invoke-Go -Arguments @('vet', './...')
    }
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    Push-Location 'cmd/copilot'
    try { Invoke-Go -Arguments @('run', 'github.com/tc-hib/go-winres@v0.3.3', 'make', '--in', 'winres.json', '--out', 'rsrc') }
    finally { Pop-Location }
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    Invoke-Go -Arguments @('build', '-trimpath', '-buildvcs=false', '-ldflags', '-s -w -H=windowsgui', '-o', 'dist/ED Copilot.exe', './cmd/copilot')
    $binary = Join-Path $projectRoot 'dist/ED Copilot.exe'
    function Invoke-Smoke {
        param([string]$Mode, [string]$ReportName, [switch]$StressRun)
        $report = Join-Path $projectRoot ('dist/' + $ReportName)
        if (Test-Path -LiteralPath $report) { Remove-Item -LiteralPath $report -Force }
        $smokeArguments = @($Mode, '--smoke-report', ('"' + $report + '"'))
        if ($StressRun) { $smokeArguments += '--stress' }
        $process = Start-Process -FilePath $binary -ArgumentList $smokeArguments -WindowStyle Hidden -PassThru
        $timeoutMs = if ($Mode -eq '--smoke') { 180000 } else { 60000 }
        if (-not $process.WaitForExit($timeoutMs)) {
            Stop-Process -Id $process.Id
            throw "Smoke timed out: $Mode"
        }
        if ($process.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $report)) { throw "Smoke failed: $Mode" }
        $result = Get-Content -LiteralPath $report -Raw | ConvertFrom-Json
        if (-not $result.ok) { throw "Smoke reported failure: $Mode" }
        if ($result.version -ne (Get-Item -LiteralPath $binary).VersionInfo.ProductVersion) { throw 'Executable resource version differs from application version.' }
    }
    Invoke-Smoke -Mode '--smoke-headless' -ReportName 'smoke-headless.json'
    if ($Smoke) {
        Invoke-Smoke -Mode '--smoke' -ReportName 'smoke-native.json'
        Invoke-Smoke -Mode '--smoke' -ReportName 'smoke-stress.json' -StressRun
    }
    $version = (Get-Item -LiteralPath $binary).VersionInfo.ProductVersion
    if ($version -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid executable product version.' }
    $artifacts = @($binary)
    if ($Package -or $Installer) {
        $stage = Join-Path $projectRoot ('dist/package-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $stage | Out-Null
        Copy-Item -LiteralPath $binary -Destination $stage
        Copy-Item -LiteralPath 'README.md' -Destination $stage
        Copy-Item -LiteralPath 'docs' -Destination $stage -Recurse
        Copy-Item -LiteralPath 'lib' -Destination $stage -Recurse
        Copy-Item -LiteralPath 'dist/smoke-headless.json' -Destination $stage
        if ($Smoke) {
            Copy-Item -LiteralPath 'dist/smoke-native.json' -Destination $stage
            Copy-Item -LiteralPath 'dist/smoke-stress.json' -Destination $stage
        }
        if ($Package) {
            $archive = Join-Path $projectRoot "dist/ED-Copilot-$version-windows-amd64.zip"
            Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -CompressionLevel Optimal -Force
            $artifacts += $archive
            Write-Output "Package: $archive"
        }
        if ($Installer) {
            . (Join-Path $projectRoot 'scripts/installer-tools.ps1')
            $installerTools = Get-EDCopilotInstallerTools -ToolsDirectory (Join-Path $projectRoot 'dist/tools')
            $setup = New-EDCopilotSetup -PayloadDirectory $stage -OutputDirectory (Join-Path $projectRoot 'dist') -Version $version -Tools $installerTools
            $artifacts += $setup
            if ($Smoke) {
                & (Join-Path $projectRoot 'scripts/installer-smoke.ps1') -PayloadDirectory $stage -OutputDirectory (Join-Path $projectRoot 'dist') -Version $version -Tools $installerTools
            }
            Write-Output "Installer: $setup"
        }
    }
    $lines = $artifacts | ForEach-Object { (Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + [IO.Path]::GetFileName($_) }
    [IO.File]::WriteAllLines((Join-Path $projectRoot 'dist/SHA256SUMS.txt'), $lines, [Text.UTF8Encoding]::new($false))
    Write-Output "Built: $binary"
}
finally {
    $env:GOOS = $savedGoEnvironment.GOOS
    $env:GOARCH = $savedGoEnvironment.GOARCH
    $env:CGO_ENABLED = $savedGoEnvironment.CGO_ENABLED
    Pop-Location
}

