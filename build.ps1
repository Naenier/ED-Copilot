#requires -Version 7.2
[CmdletBinding()]
param([switch]$Smoke, [switch]$Package, [switch]$Installer, [switch]$SkipTests, [string]$SourceCommit=$env:GITHUB_SHA)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'This build targets native Windows x64 controls.' }
$projectRoot = $PSScriptRoot
. (Join-Path $projectRoot 'scripts/delivery-lib.ps1')
$applicationVersion=Get-EDCopilotVersion $projectRoot
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
        $frontendTests=@(Get-ChildItem -LiteralPath 'internal/frontend' -Filter '*.test.cjs' -File | Sort-Object Name | ForEach-Object FullName)
        if($frontendTests.Count -eq 0){throw 'Frontend tests were not found.'}
        & node --test @frontendTests
        if($LASTEXITCODE -ne 0){throw 'Frontend tests failed.'}
        & (Join-Path $projectRoot 'scripts/delivery-tests.ps1')
        Invoke-Go -Arguments @('test', './...', '-count=1', '-timeout=120s')
        Invoke-Go -Arguments @('vet', './...')
    }
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    $releaseManifest=Join-Path $projectRoot 'dist/release-manifest.json'
    if(Test-Path -LiteralPath $releaseManifest){Remove-Item -LiteralPath $releaseManifest}
    Push-Location 'cmd/copilot'
    try {
        # app.Version is the single version source. Resource JSON is a template.
        $resource=Get-Content -LiteralPath 'winres.json' -Raw|ConvertFrom-Json -AsHashtable
        $entry=$resource['RT_VERSION']['#1']['0000']
        $entry.fixed.file_version="$applicationVersion.0"
        $entry.fixed.product_version="$applicationVersion.0"
        foreach($language in $entry.info.Values){$language.FileVersion=$applicationVersion;$language.ProductVersion=$applicationVersion}
        Write-EDDeliveryJSON 'winres.generated.json' $resource
        Invoke-Go -Arguments @('run', 'github.com/tc-hib/go-winres@v0.3.3', 'make', '--in', 'winres.generated.json', '--out', 'rsrc')
    }
    finally { if(Test-Path -LiteralPath 'winres.generated.json'){Remove-Item -LiteralPath 'winres.generated.json'}; Pop-Location }
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
        if (-not $result.ok) {
            # These reports belong to the isolated, network-disabled profile.
            # Preserve the actual failing checks in CI instead of only a mode.
            if ($result.PSObject.Properties.Name -contains 'scenarios') {
                $result.scenarios | Where-Object { $_.ok -eq $false } | ConvertTo-Json -Depth 6 | Write-Output
            }
            foreach ($property in $result.PSObject.Properties) {
                if ($property.Name -match 'error|fail|problem') { Write-Output ($property.Name + ': ' + ($property.Value | ConvertTo-Json -Depth 6 -Compress)) }
            }
            throw "Smoke reported failure: $Mode"
        }
        if ($result.version -ne (Get-Item -LiteralPath $binary).VersionInfo.ProductVersion) { throw 'Executable resource version differs from application version.' }
    }
    Invoke-Smoke -Mode '--smoke-headless' -ReportName 'smoke-headless.json'
    if ($Smoke) {
        Invoke-Smoke -Mode '--smoke' -ReportName 'smoke-native.json'
        Invoke-Smoke -Mode '--smoke' -ReportName 'smoke-stress.json' -StressRun
    }
    $version = (Get-Item -LiteralPath $binary).VersionInfo.ProductVersion
    if ($version -ne $applicationVersion) { throw 'Executable product version differs from app.Version.' }
    $artifacts = @($binary)
    if ($Package -or $Installer) {
        $stage = Join-Path $projectRoot ('dist/package-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $stage | Out-Null
        Copy-Item -LiteralPath $binary -Destination $stage
        Copy-Item -LiteralPath 'README.md' -Destination $stage
        Copy-Item -LiteralPath 'README.ru.md' -Destination $stage
        Copy-Item -LiteralPath 'docs' -Destination $stage -Recurse
        Copy-Item -LiteralPath 'lib' -Destination $stage -Recurse
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
    if($Smoke -and $Package -and $Installer){
        $null=New-EDReleaseManifest -Root $projectRoot -Directory (Join-Path $projectRoot 'dist') -Commit $SourceCommit -TestsPassed (-not $SkipTests)
    }
    Write-Output "Built: $binary"
}
finally {
    $env:GOOS = $savedGoEnvironment.GOOS
    $env:GOARCH = $savedGoEnvironment.GOARCH
    $env:CGO_ENABLED = $savedGoEnvironment.CGO_ENABLED
    Pop-Location
}

