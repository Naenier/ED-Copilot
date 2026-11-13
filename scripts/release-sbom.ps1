# Helpers for a CycloneDX inventory derived from the packaged binaries.

function New-EDSBOMFileComponent {
    param([string]$Path, [string]$Name, [string]$Version, [string]$Reference)
    $digest = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    [ordered]@{ type='application'; name=$Name; version=$Version; 'bom-ref'=$Reference; hashes=@(@{alg='SHA-256';content=$digest}) }
}

function New-EDApplicationSBOM {
    param([string]$Executable, [string]$Version, [System.Collections.IDictionary]$BuildInfo)
    if ($BuildInfo.Path -cne 'github.com/Naenier/ED-Copilot/cmd/copilot' -or $BuildInfo.GoVersion -notmatch '^go[0-9]+\.[0-9]+\.[0-9]+$') {
        throw 'Unexpected executable Go build information.'
    }
    $settings = @{}
    foreach ($setting in $BuildInfo.Settings) { $settings[$setting.Key] = $setting.Value }
    if ($settings.GOOS -ne 'windows' -or $settings.GOARCH -ne 'amd64' -or $settings.CGO_ENABLED -ne '0') {
        throw 'Unexpected executable Go build target.'
    }
    $application = New-EDSBOMFileComponent $Executable 'ED Copilot' $Version 'ed-copilot'
    $components = @(
        [ordered]@{type='library'; name='Go standard library'; version=$BuildInfo.GoVersion.Substring(2); 'bom-ref'='go:stdlib'; purl="pkg:golang/stdlib@$($BuildInfo.GoVersion)"}
        foreach ($dependency in $BuildInfo.Deps | Sort-Object { $_.Path } -CaseSensitive) {
            if ($dependency.Contains('Replace') -or $dependency.Path -notmatch '^[A-Za-z0-9._~-]+(?:/[A-Za-z0-9._~-]+)+$' -or
                $dependency.Version -notmatch '^v[0-9][A-Za-z0-9.+-]*$' -or $dependency.Sum -notmatch '^h1:[A-Za-z0-9+/]{43}=$') {
                throw 'An embedded Go dependency lacks an exact, unreplaced module identity and checksum.'
            }
            $purl = 'pkg:golang/' + $dependency.Path + '@' + [uri]::EscapeDataString($dependency.Version)
            [ordered]@{
                type='library'; name=$dependency.Path; version=$dependency.Version; 'bom-ref'=$purl; purl=$purl
                properties=@(@{name='ed-copilot:go-module-h1'; value=$dependency.Sum})
            }
        }
    )
    [ordered]@{
        '$schema'='https://cyclonedx.org/schema/bom-1.6.schema.json'; bomFormat='CycloneDX'; specVersion='1.6'; version=1
        serialNumber=('urn:uuid:' + [Guid]::NewGuid().ToString('D'))
        metadata=@{component=$application}; components=$components
        dependencies=@(@{ref=$application['bom-ref']; dependsOn=@($components | ForEach-Object { $_['bom-ref'] })})
    }
}

function Write-EDReleaseSBOMs {
    param([string]$Executable, [string]$Setup, [string]$Version, $InstallerTools, [string]$Directory)
    # Inno Setup reserves padding in its version resource.
    $executableVersion = [string](Get-Item -LiteralPath $Executable).VersionInfo.ProductVersion
    $setupVersion = [string](Get-Item -LiteralPath $Setup).VersionInfo.ProductVersion
    if ($executableVersion.Trim() -cne $Version -or $setupVersion.Trim() -cne $Version) {
        throw 'The packaged application and installer must use the same release version.'
    }
    $buildInfoJSON = & go version -m -json $Executable
    if ($LASTEXITCODE) { throw 'Could not read the executable Go build information.' }
    $application = New-EDApplicationSBOM $Executable $Version (($buildInfoJSON -join "`n") | ConvertFrom-Json -AsHashtable)
    $bootstrapperVersion = (Get-Item -LiteralPath $InstallerTools.Bootstrapper).VersionInfo.ProductVersion
    if (-not $bootstrapperVersion) { throw 'The bundled WebView2 bootstrapper has no version.' }
    $bootstrapper = New-EDSBOMFileComponent $InstallerTools.Bootstrapper 'Microsoft Edge WebView2 bootstrapper' $bootstrapperVersion 'webview2-bootstrapper'
    $inno = [ordered]@{type='library'; name='Inno Setup engine'; version=$InstallerTools.Version; 'bom-ref'='inno-setup'}
    $setupComponent = New-EDSBOMFileComponent $Setup 'ED Copilot Setup' $Version 'ed-copilot-setup'
    $payload = $application.metadata.component
    $installer = [ordered]@{
        '$schema'=$application['$schema']; bomFormat=$application.bomFormat; specVersion=$application.specVersion; version=$application.version
        serialNumber=('urn:uuid:' + [Guid]::NewGuid().ToString('D'))
        metadata=@{component=$setupComponent}
        components=@($payload, $bootstrapper, $inno) + $application.components
        dependencies=@(@{ref=$setupComponent['bom-ref']; dependsOn=@($payload['bom-ref'], $bootstrapper['bom-ref'], $inno['bom-ref'])}) + $application.dependencies
    }
    Write-EDDeliveryJSON (Join-Path $Directory 'application.cdx.json') $application
    Write-EDDeliveryJSON (Join-Path $Directory 'installer.cdx.json') $installer
}
