[CmdletBinding()]
param([switch]$Smoke, [switch]$Package, [switch]$SkipTests)
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
        param([string]$Mode, [string]$ReportName)
        $report = Join-Path $projectRoot ('dist/' + $ReportName)
        $process = Start-Process -FilePath $binary -ArgumentList @($Mode, '--smoke-report', ('"' + $report + '"')) -WindowStyle Hidden -PassThru
        if (-not $process.WaitForExit(60000)) {
            Stop-Process -Id $process.Id
            throw "Smoke timed out: $Mode"
        }
        if ($process.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $report)) { throw "Smoke failed: $Mode" }
        $result = Get-Content -LiteralPath $report -Raw | ConvertFrom-Json
        if (-not $result.ok) { throw "Smoke reported failure: $Mode" }
    }
    Invoke-Smoke -Mode '--smoke-headless' -ReportName 'smoke-headless.json'
    if ($Smoke) { Invoke-Smoke -Mode '--smoke' -ReportName 'smoke-native.json' }
    if ($Package) {
        $version = (Get-Item -LiteralPath $binary).VersionInfo.ProductVersion
        if ($version -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid executable product version.' }
        $stage = Join-Path $projectRoot ('dist/package-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $stage | Out-Null
        Copy-Item -LiteralPath $binary -Destination $stage
        Copy-Item -LiteralPath 'README.md' -Destination $stage
        Copy-Item -LiteralPath 'docs' -Destination $stage -Recurse
        Copy-Item -LiteralPath 'lib' -Destination $stage -Recurse
        Copy-Item -LiteralPath 'dist/smoke-headless.json' -Destination $stage
        if ($Smoke) { Copy-Item -LiteralPath 'dist/smoke-native.json' -Destination $stage }
        $archive = Join-Path $projectRoot "dist/ED-Copilot-$version-windows-amd64.zip"
        Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -CompressionLevel Optimal -Force
        $lines = @($binary, $archive) | ForEach-Object { (Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + [IO.Path]::GetFileName($_) }
        [IO.File]::WriteAllLines((Join-Path $projectRoot 'dist/SHA256SUMS.txt'), $lines, [Text.UTF8Encoding]::new($false))
        Write-Output "Package: $archive"
    }
    Write-Output "Built: $binary"
}
finally {
    $env:GOOS = $savedGoEnvironment.GOOS
    $env:GOARCH = $savedGoEnvironment.GOARCH
    $env:CGO_ENABLED = $savedGoEnvironment.CGO_ENABLED
    Pop-Location
}

