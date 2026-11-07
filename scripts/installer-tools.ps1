# Build tools are kept inside dist; they are not installed into Windows.
$ErrorActionPreference = 'Stop'

function Get-EDCopilotInstallerTools {
    param([Parameter(Mandatory=$true)][string]$ToolsDirectory)
    $toolRoot = [IO.Path]::GetFullPath($ToolsDirectory)
    New-Item -ItemType Directory -Path $toolRoot -Force | Out-Null
    $innoVersion = '7.1.1'
    $innoSHA256 = '8a5a6711c3766a6e5fb74f280ba0e3551e719ce94538d901fdbd1f8f9920b3f0'
    $archive = Join-Path $toolRoot "innosetup-$innoVersion-x64.exe"
    $innoRoot = Join-Path $toolRoot "inno-$innoVersion"
    $compiler = Join-Path $innoRoot 'ISCC.exe'
    if (-not (Test-Path -LiteralPath $archive)) {
        Invoke-WebRequest -Uri 'https://github.com/jrsoftware/issrc/releases/download/is-7_1_1/innosetup-7.1.1-x64.exe' -OutFile $archive
    }
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $innoSHA256) { throw 'Inno Setup download checksum is invalid.' }
    $signature = Get-AuthenticodeSignature -LiteralPath $archive
    if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch '(^|,\s*)O=Pyrsys B\.V\.(,|$)') {
        throw 'Inno Setup must have a valid Pyrsys B.V. Authenticode signature.'
    }
    if (-not (Test-Path -LiteralPath $compiler)) {
        $process = Start-Process -FilePath $archive -ArgumentList @('/PORTABLE=1', '/CURRENTUSER', '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', '/NOICONS', ('/DIR="' + $innoRoot + '"')) -WindowStyle Hidden -PassThru
        if (-not $process.WaitForExit(60000)) { Stop-Process -Id $process.Id; throw 'Inno Setup portable extraction timed out.' }
        if ($process.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $compiler)) { throw 'Inno Setup portable extraction failed.' }
    }
    $bootstrapper = Join-Path $toolRoot 'MicrosoftEdgeWebview2Setup.exe'
    if (-not (Test-Path -LiteralPath $bootstrapper)) {
        Invoke-WebRequest -Uri 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $bootstrapper
    }
    $runtimeSignature = Get-AuthenticodeSignature -LiteralPath $bootstrapper
    if ($runtimeSignature.Status -ne 'Valid' -or $runtimeSignature.SignerCertificate.Subject -notmatch '(^|,\s*)O=Microsoft Corporation(,|$)') {
        throw 'The WebView2 bootstrapper must have a valid Microsoft Corporation Authenticode signature.'
    }
    [pscustomobject]@{ Compiler = $compiler; Bootstrapper = $bootstrapper; Version = $innoVersion }
}

function New-EDCopilotSetup {
    param(
        [Parameter(Mandatory=$true)][string]$PayloadDirectory,
        [Parameter(Mandatory=$true)][string]$OutputDirectory,
        [Parameter(Mandatory=$true)][string]$Version,
        [Parameter(Mandatory=$true)][object]$Tools,
        [string]$SmokeIdentifier = '',
        [switch]$SimulateMissingRuntime
    )
    if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid setup version.' }
    if ($SmokeIdentifier -and $SmokeIdentifier -notmatch '^[0-9a-f]{32}$') { throw 'Invalid isolated installer identifier.' }
    if ($SimulateMissingRuntime -and -not $SmokeIdentifier) { throw 'Missing-runtime simulation is restricted to the isolated smoke installer.' }
    $payload = (Resolve-Path -LiteralPath $PayloadDirectory).ProviderPath
    $output = [IO.Path]::GetFullPath($OutputDirectory)
    New-Item -ItemType Directory -Path $output -Force | Out-Null
    $installerName = "ED-Copilot-$Version-Setup"
    if ($SmokeIdentifier) { $installerName += '-smoke' }
    if ($SimulateMissingRuntime) { $installerName += '-missing-runtime' }
    $arguments = @('/Qp', ('/DApplicationVersion=' + $Version), ('/DPayloadDir=' + $payload), ('/DOutputDir=' + $output), ('/DInstallerFileName=' + $installerName), ('/DWebViewBootstrapper=' + $Tools.Bootstrapper))
    if ($SmokeIdentifier) { $arguments += '/DTestAppId=' + $SmokeIdentifier }
    if ($SimulateMissingRuntime) { $arguments += '/DTestMissingRuntime=1' }
    $arguments += Join-Path $PSScriptRoot 'installer.iss'
    & $Tools.Compiler @arguments | ForEach-Object { Write-Host $_ }
    if ($LASTEXITCODE -ne 0) { throw "Inno Setup compilation failed ($LASTEXITCODE)." }
    $setup = Join-Path $output ($installerName + '.exe')
    if (-not (Test-Path -LiteralPath $setup)) { throw 'Inno Setup did not produce the expected installer.' }
    return $setup
}
