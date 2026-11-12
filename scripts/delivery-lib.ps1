# Shared by build, CI publication, connector preparation and local delivery.
# No commands run when this file is dot-sourced.
Set-StrictMode -Version Latest
$script:EDDeliveryRepository = 'Naenier/ED-Copilot'
$script:EDSmokeKinds = @('headless','native','stress','installer')
$script:EDReleaseSBOMNames = @('application.cdx.json','installer.cdx.json')

function Get-EDPublicArtifactNames {
    param([string]$Version,[int]$SchemaVersion)
    "ED-Copilot-$Version-Setup.exe"
    "ED-Copilot-$Version-windows-amd64.zip"
    'SHA256SUMS.txt'
    if ($SchemaVersion -eq 1) { $script:EDSmokeKinds | ForEach-Object { "smoke-$_.json" } }
}

function Get-EDSafePath {
    param([Parameter(Mandatory)][string]$Path)
    $full=$ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Path)
    if(-not [IO.Path]::IsPathFullyQualified($full)){throw 'Delivery requires a filesystem path.'}
    $full=[IO.Path]::GetFullPath($full)
    for($part=$full;$part;$part=[IO.Path]::GetDirectoryName($part)){
        if((Test-Path -LiteralPath $part) -and ((Get-Item -LiteralPath $part -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Delivery path contains a symbolic link or junction.'}
    }
    return $full
}

function Test-EDPathOverlap {
    param([string]$A,[string]$B)
    $aPath=(Get-EDSafePath $A).TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar
    $bPath=(Get-EDSafePath $B).TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar
    return $aPath.StartsWith($bPath,[StringComparison]::OrdinalIgnoreCase) -or $bPath.StartsWith($aPath,[StringComparison]::OrdinalIgnoreCase)
}

function Get-EDCopilotVersion {
    param([Parameter(Mandatory)][string]$Root)
    $matches = [regex]::Matches([IO.File]::ReadAllText((Get-EDSafePath (Join-Path $Root 'internal/app/engine.go'))), 'const Version = "((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))"')
    if ($matches.Count -ne 1) { throw 'A single canonical app.Version is required.' }
    return $matches[0].Groups[1].Value
}

function Write-EDDeliveryJSON {
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)]$Value)
    $Path=Get-EDSafePath $Path
    $parent = [IO.Path]::GetDirectoryName($Path)
    [IO.Directory]::CreateDirectory($parent) | Out-Null
    $partial=$Path+'.'+[guid]::NewGuid().ToString('N')+'.partial'
    $bytes=[Text.Encoding]::UTF8.GetBytes(($Value|ConvertTo-Json -Depth 30))
    try{
        $out=[IO.File]::Open($partial,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try{$out.Write($bytes,0,$bytes.Length)}finally{$out.Dispose()}
        [IO.File]::Move($partial,$Path,$true)
    }finally{if([IO.File]::Exists($partial)){[IO.File]::Delete($partial)}}
}

function Get-EDSourceState {
    param([Parameter(Mandatory)][string]$Root)
    $rootPath = (Resolve-Path -LiteralPath $Root).ProviderPath.TrimEnd('\','/')
    $directories = @('internal','cmd','docs','lib','scripts','.github')
    $rootFiles = @('.gitattributes','.gitignore','README.md','README.ru.md','go.mod','go.sum','build.ps1')
    $candidates = @($rootFiles | ForEach-Object { Join-Path $rootPath $_ } | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf })
    foreach ($directory in $directories) {
        $path = Join-Path $rootPath $directory
        if (Test-Path -LiteralPath $path) { $candidates += @(Get-ChildItem -LiteralPath $path -File -Recurse | ForEach-Object FullName) }
    }
    $byPath = @{}
    foreach ($path in $candidates) {
        $relative = $path.Substring($rootPath.Length+1).Replace('\','/')
        if ($relative -match '(^|/)(node_modules|WebView2|updates|\.git)(/|$)' -or $relative -match '\.(syso|log|prof|test\.exe)$' -or $relative.EndsWith('winres.generated.json')) { continue }
        $byPath[$relative] = $path
    }
    $names = [string[]]@($byPath.Keys)
    [Array]::Sort($names,[StringComparer]::Ordinal)
    $files = [Collections.Generic.List[object]]::new()
    $digestLines = [Text.StringBuilder]::new()
    foreach ($name in $names) {
        if(([IO.File]::GetAttributes($byPath[$name]) -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Source publication refuses symbolic links.'}
        $bytes = [IO.File]::ReadAllBytes($byPath[$name])
        $sha256 = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
        $header = [Text.Encoding]::ASCII.GetBytes(('blob '+$bytes.Length+[char]0))
        $hasher = [Security.Cryptography.SHA1]::Create()
        try { $null=$hasher.TransformBlock($header,0,$header.Length,$null,0); $null=$hasher.TransformFinalBlock($bytes,0,$bytes.Length); $sha=[Convert]::ToHexString($hasher.Hash).ToLowerInvariant() } finally { $hasher.Dispose() }
        $text = $true
        $canonicalSHA=$sha256
        try {
            $decoded=[Text.UTF8Encoding]::new($false,$true).GetString($bytes)
            $canonicalSHA=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($decoded.Replace("`r`n","`n")))).ToLowerInvariant()
        } catch { $text=$false }
        $files.Add([ordered]@{path=$name;bytes=$bytes.Length;sha=$sha;sha256=$sha256;text=$text})
        $null=$digestLines.Append($name).Append([char]0).Append($canonicalSHA).Append("`n")
    }
    $treeSHA256 = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($digestLines.ToString()))).ToLowerInvariant()
    return [pscustomobject]@{files=$files.ToArray();treeSHA256=$treeSHA256}
}

function Assert-EDSmokeReport {
    param([Parameter(Mandatory)]$Report,[Parameter(Mandatory)][string]$Version,[Parameter(Mandatory)][string]$Kind)
    if ($Report.ok -isnot [bool] -or -not $Report.ok -or $Report.version -ne $Version -or $Report.networkDisabled -isnot [bool] -or -not $Report.networkDisabled) { throw "Invalid $Kind smoke report." }
    if ($Kind -in @('native','stress') -and ($Report.webviewGeometry.ok -isnot [bool] -or -not $Report.webviewGeometry.ok)) { throw "Invalid $Kind geometry report." }
    if ($Kind -eq 'installer' -and ($Report.profileUnchanged -isnot [bool] -or -not $Report.profileUnchanged)) { throw 'Installer did not preserve the isolated profile.' }
}

function New-EDReleaseManifest {
    param([Parameter(Mandatory)][string]$Root,[Parameter(Mandatory)][string]$Directory,[string]$Commit='', [bool]$TestsPassed=$false)
    $version = Get-EDCopilotVersion $Root
    if ($Commit -and $Commit -notmatch '^[0-9a-f]{40}$') { throw 'Invalid source commit.' }
    $source = Get-EDSourceState $Root
    $schemaVersion = 3
    $published = @(Get-EDPublicArtifactNames $version $schemaVersion)
    $reports = @($script:EDSmokeKinds | ForEach-Object { "smoke-$_.json" })
    $names = @('ED Copilot.exe') + $published + $reports + $script:EDReleaseSBOMNames
    $artifacts = @()
    foreach ($name in $names) {
        $file = Join-Path $Directory $name
        if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Release artifact missing: $name" }
        $artifacts += [ordered]@{name=$name;bytes=(Get-Item -LiteralPath $file).Length;sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant();published=$name -in $published}
    }
    foreach ($kind in $script:EDSmokeKinds) { Assert-EDSmokeReport (Get-Content -LiteralPath (Join-Path $Directory "smoke-$kind.json") -Raw | ConvertFrom-Json) $version $kind }
    $manifest = [ordered]@{schemaVersion=$schemaVersion;version=$version;sourceCommit=$Commit;sourceTreeSHA256=$source.treeSHA256;sourceDigestFormat='paths+text-lf-sha256-v1';sourceFiles=$source.files.Count;platform='windows';architecture='amd64';testsPassed=$TestsPassed;artifacts=$artifacts}
    Write-EDDeliveryJSON (Join-Path $Directory 'release-manifest.json') $manifest
    return $manifest
}

function Read-EDChecksums {
    param([Parameter(Mandatory)][string]$Path)
    $sums = @{}
    foreach ($line in [IO.File]::ReadAllLines($Path)) {
        if ($line -notmatch '^([0-9a-f]{64})  ([^/\\]+)$' -or $sums.ContainsKey($Matches[2])) { throw 'Malformed or duplicate release checksum.' }
        $sums[$Matches[2]]=$Matches[1]
    }
    return $sums
}

function Assert-EDSBOMComponent {
    param($Component,[string]$Reference,[string]$Version,[string]$SHA256)
    if (-not $Component -or $Component['bom-ref'] -cne $Reference -or $Component.version -cne $Version -or
        @($Component.hashes | Where-Object { $_.alg -ceq 'SHA-256' -and $_.content -ceq $SHA256 }).Count -ne 1) {
        throw 'SBOM component does not match the release executable.'
    }
}

function Assert-EDReleaseSBOMs {
    param([string]$Directory,[string]$Version,$Entries)
    $executable = $Entries['ED Copilot.exe'].sha256
    $setup = $Entries["ED-Copilot-$Version-Setup.exe"].sha256
    foreach ($name in $script:EDReleaseSBOMNames) {
        $path = Join-Path $Directory $name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { continue }
        $bom = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json -AsHashtable
        if ($bom.bomFormat -cne 'CycloneDX' -or $bom.specVersion -cne '1.6') { throw "Unsupported SBOM format: $name" }
        if ($name -eq 'application.cdx.json') {
            Assert-EDSBOMComponent $bom.metadata.component 'ed-copilot' $Version $executable
        } else {
            Assert-EDSBOMComponent $bom.metadata.component 'ed-copilot-setup' $Version $setup
            $payload = @($bom.components | Where-Object { $_['bom-ref'] -ceq 'ed-copilot' })
            if ($payload.Count -ne 1) { throw 'Installer SBOM lacks the executable component.' }
            Assert-EDSBOMComponent $payload[0] 'ed-copilot' $Version $executable
        }
    }
}

function Test-EDReleaseDirectory {
    param([Parameter(Mandatory)][string]$Directory,[Parameter(Mandatory)][string]$ExpectedCommit,[string]$SourceRoot='', [Alias('RequireSmokeReports')][switch]$RequireBuildEvidence)
    if ($ExpectedCommit -notmatch '^[0-9a-f]{40}$') { throw 'A full expected source commit is required.' }
    $Directory=Get-EDSafePath $Directory
    $m = Get-Content -LiteralPath (Join-Path $Directory 'release-manifest.json') -Raw | ConvertFrom-Json
    if ($m.schemaVersion -notin @(1,2,3) -or $m.version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' -or $m.sourceCommit -ne $ExpectedCommit -or $m.sourceTreeSHA256 -notmatch '^[0-9a-f]{64}$' -or $m.sourceDigestFormat -ne 'paths+text-lf-sha256-v1' -or $m.platform -ne 'windows' -or $m.architecture -ne 'amd64' -or $m.testsPassed -isnot [bool] -or -not $m.testsPassed) { throw 'Release provenance or validation manifest is invalid.' }
    if ($SourceRoot -and ((Get-EDSourceState $SourceRoot).treeSHA256 -ne $m.sourceTreeSHA256 -or (Get-EDCopilotVersion $SourceRoot) -ne $m.version)) { throw 'Release does not match the checked-out source.' }
    $published = @(Get-EDPublicArtifactNames $m.version $m.schemaVersion)
    $reports = @($script:EDSmokeKinds | ForEach-Object { "smoke-$_.json" })
    $evidence = $reports
    if ($m.schemaVersion -eq 3) { $evidence += $script:EDReleaseSBOMNames }
    $expected = @('ED Copilot.exe') + @(Get-EDPublicArtifactNames $m.version 2) + $evidence
    $entries=@{}
    foreach ($a in $m.artifacts) {
        if ($a.name -notin $expected -or $entries.ContainsKey($a.name) -or $a.bytes -le 0 -or $a.bytes -gt 268435456 -or $a.sha256 -notmatch '^[0-9a-f]{64}$' -or $a.published -isnot [bool] -or $a.published -ne ($a.name -in $published)) { throw 'Invalid or duplicate artifact manifest entry.' }
        $entries[$a.name]=$a
        $path=Get-EDSafePath (Join-Path $Directory $a.name)
        if (Test-Path -LiteralPath $path -PathType Leaf) {
            if ((Get-Item -LiteralPath $path).Length -ne $a.bytes -or (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $a.sha256) { throw "Artifact hash/size mismatch: $($a.name)" }
        } elseif ($a.published -or ($RequireBuildEvidence -and $a.name -in $evidence)) { throw "Missing release artifact: $($a.name)" }
    }
    if ($entries.Count -ne $expected.Count) { throw 'Incomplete release manifest.' }
    if ($m.schemaVersion -eq 3) { Assert-EDReleaseSBOMs $Directory $m.version $entries }
    $sums=Read-EDChecksums (Join-Path $Directory 'SHA256SUMS.txt')
    foreach ($name in @('ED Copilot.exe',"ED-Copilot-$($m.version)-Setup.exe","ED-Copilot-$($m.version)-windows-amd64.zip")) { if ($sums[$name] -ne $entries[$name].sha256) { throw 'Release checksum sources disagree.' } }
    foreach ($kind in $script:EDSmokeKinds) {
        $report = Join-Path $Directory "smoke-$kind.json"
        if (Test-Path -LiteralPath $report -PathType Leaf) { Assert-EDSmokeReport (Get-Content -LiteralPath $report -Raw | ConvertFrom-Json) $m.version $kind }
    }
    # Inspect the one executable entry as a stream; never extract arbitrary ZIP paths.
    $zip=[IO.Compression.ZipFile]::OpenRead((Join-Path $Directory "ED-Copilot-$($m.version)-windows-amd64.zip"))
    try {
        if ($m.schemaVersion -ge 2 -and @($zip.Entries | Where-Object Name -In $evidence).Count) { throw 'Portable package includes CI-only evidence.' }
        $found=@($zip.Entries | Where-Object FullName -EQ 'ED Copilot.exe')
        if ($found.Count -ne 1 -or $found[0].Length -ne $entries['ED Copilot.exe'].bytes) { throw 'Portable executable is missing or duplicated.' }
        $stream=$found[0].Open();$hash=[Security.Cryptography.SHA256]::Create()
        try { $digest=[Convert]::ToHexString($hash.ComputeHash($stream)).ToLowerInvariant() } finally { $stream.Dispose();$hash.Dispose() }
        if ($digest -ne $entries['ED Copilot.exe'].sha256) { throw 'Portable executable identity mismatch.' }
    } finally { $zip.Dispose() }
    return $m
}

function Assert-EDReleaseAttestations {
    param([string]$Directory,$Manifest,[scriptblock]$Verify={
        param($Path,$Predicate,$Commit)
        & gh attestation verify $Path --repo $script:EDDeliveryRepository --signer-workflow "$script:EDDeliveryRepository/.github/workflows/windows.yml" --source-ref refs/heads/main --source-digest $Commit --deny-self-hosted-runners --predicate-type $Predicate
        if ($LASTEXITCODE -ne 0) { throw "Could not verify GitHub attestation: $([IO.Path]::GetFileName($Path))" }
    })
    if ($Manifest.schemaVersion -ne 3) { throw 'Publication requires the release manifest with SBOM evidence.' }
    $provenance = 'https://slsa.dev/provenance/v1'
    $sbom = 'https://cyclonedx.org/bom'
    $publicNames = @(Get-EDPublicArtifactNames $Manifest.version $Manifest.schemaVersion)
    foreach ($name in $publicNames + 'release-manifest.json') {
        & $Verify (Join-Path $Directory $name) $provenance $Manifest.sourceCommit
    }
    foreach ($name in $publicNames | Where-Object { $_ -match '\.(exe|zip)$' }) {
        & $Verify (Join-Path $Directory $name) $sbom $Manifest.sourceCommit
    }
}

function Assert-EDPublicURL {
    param([Parameter(Mandatory)][uri]$URL)
    if ($URL.Scheme -ne 'https' -or $URL.UserInfo -or -not $URL.IsDefaultPort) { throw 'Untrusted delivery URL.' }
    switch ($URL.DnsSafeHost) {
        'api.github.com' { if (-not $URL.AbsolutePath.StartsWith('/repos/'+$script:EDDeliveryRepository+'/',[StringComparison]::Ordinal)) { throw 'Untrusted GitHub API resource.' }; return }
        'github.com' { if (-not $URL.AbsolutePath.StartsWith('/'+$script:EDDeliveryRepository+'/releases/download/v',[StringComparison]::Ordinal)) { throw 'Untrusted GitHub release resource.' }; return }
        'release-assets.githubusercontent.com' { return }
        'objects.githubusercontent.com' { return }
        default { throw 'Untrusted delivery host.' }
    }
}

function Invoke-EDPublicDownload {
    param([Parameter(Mandatory)][string]$URL,[string]$Destination='', [long]$MaxBytes=1048576,[switch]$PublisherMetadata)
    $current=[uri]$URL
    $timeout=[TimeSpan]::FromMinutes(5)
    if($PublisherMetadata){
        Assert-EDPublicURL $current
        if($Destination -or $current.DnsSafeHost -ne 'api.github.com'){throw 'Publication credentials are restricted to GitHub API metadata.'}
        if(-not $env:GH_TOKEN){throw 'GitHub publication metadata requires the workflow token.'}
        $timeout=[TimeSpan]::FromSeconds(30)
    }
    $handler=[Net.Http.HttpClientHandler]::new();$handler.AllowAutoRedirect=$false
    $client=[Net.Http.HttpClient]::new($handler);$client.Timeout=$timeout
    $cancel=[Threading.CancellationTokenSource]::new($timeout)
    $partial=''
    if($Destination){$Destination=Get-EDSafePath $Destination;$partial=$Destination+'.'+[guid]::NewGuid().ToString('N')+'.partial'}
    $client.DefaultRequestHeaders.UserAgent.ParseAdd('ED-Copilot-Delivery/1')
    $client.DefaultRequestHeaders.Accept.ParseAdd('application/vnd.github+json')
    $null=$client.DefaultRequestHeaders.TryAddWithoutValidation('X-GitHub-Api-Version','2022-11-28')
    try {
        if($PublisherMetadata){
            $client.DefaultRequestHeaders.Authorization=[Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer',$env:GH_TOKEN)
            $client.DefaultRequestHeaders.CacheControl=[Net.Http.Headers.CacheControlHeaderValue]::new()
            $client.DefaultRequestHeaders.CacheControl.NoCache=$true
        }
        for ($redirect=0;$redirect -le 4;$redirect++) {
            Assert-EDPublicURL $current
            $response=$client.GetAsync($current,[Net.Http.HttpCompletionOption]::ResponseHeadersRead,$cancel.Token).GetAwaiter().GetResult()
            try {
                $status=[int]$response.StatusCode
                if ($status -in @(301,302,303,307,308)) {
                    if($PublisherMetadata){throw 'Authenticated GitHub metadata redirects are refused.'}
                    if ($null -eq $response.Headers.Location -or $redirect -eq 4) { throw 'Invalid update redirect.' }
                    $current=[uri]::new($current,$response.Headers.Location);continue
                }
                if ($status -ne 200) { throw "GitHub download returned HTTP $status." }
                if ($response.Content.Headers.ContentLength -gt $MaxBytes) { throw 'Delivery response exceeds size limit.' }
                $input=$response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
                $output=if($Destination){[IO.File]::Open($partial,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)}else{[IO.MemoryStream]::new()}
                try {
                    $buffer=[byte[]]::new(65536);$count=[long]0
                    while (($n=$input.ReadAsync($buffer,0,$buffer.Length,$cancel.Token).GetAwaiter().GetResult()) -gt 0) { $count+=$n;if($count -gt $MaxBytes){throw 'Delivery response exceeds size limit.'};$output.Write($buffer,0,$n) }
                    if (-not $Destination) { return ,$output.ToArray() }
                } finally { $input.Dispose();$output.Dispose() }
                [IO.File]::Move($partial,(Get-EDSafePath $Destination),$true)
                return
            } finally { $response.Dispose() }
        }
    } finally {
        $client.Dispose();$handler.Dispose();$cancel.Dispose()
        if($partial -and [IO.File]::Exists($partial)){[IO.File]::Delete($partial)}
    }
}

function Get-EDPublicJSON {
    param([Parameter(Mandatory)][string]$Path,[scriptblock]$Request={param($url) Invoke-EDPublicDownload -URL $url})
    $url='https://api.github.com/repos/'+$script:EDDeliveryRepository+'/'+$Path
    Assert-EDPublicURL ([uri]$url)
    $bytes=& $Request $url
    return [Text.Encoding]::UTF8.GetString([byte[]]$bytes) | ConvertFrom-Json
}

function Get-EDInstallDecision {
    param([string]$InstalledVersion,[string]$InstalledSHA256,[Parameter(Mandatory)][string]$TargetVersion,[Parameter(Mandatory)][string]$TargetSHA256,[switch]$AllowSameVersionRepair)
    if (-not $InstalledVersion) { return 'install' }
    if ([version]$InstalledVersion -gt [version]$TargetVersion) { throw 'Refusing an automatic downgrade.' }
    if ($InstalledVersion -eq $TargetVersion) {
        if ($InstalledSHA256 -eq $TargetSHA256) { return 'current' }
        if (-not $AllowSameVersionRepair) { throw 'Same version has different executable bytes; explicit verified repair is required.' }
        return 'repair'
    }
    return 'upgrade'
}

function Get-EDCriticalProfile {
    param([Parameter(Mandatory)][string]$Directory)
    $root=(Get-EDSafePath $Directory).TrimEnd('\','/')
    $digests=@{}
    if(Test-Path -LiteralPath $root){
        foreach($file in Get-ChildItem -LiteralPath $root -File -Recurse){
            $relative=$file.FullName.Substring($root.Length+1).Replace('\','/')
            if($relative -match '^(WebView2|updates)/'){continue}
            $digests[$relative]=(Get-FileHash -LiteralPath (Get-EDSafePath $file.FullName) -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    return $digests
}

function Assert-EDCriticalProfile {
    param([Parameter(Mandatory)][hashtable]$Before,[Parameter(Mandatory)][string]$Directory)
    $after=Get-EDCriticalProfile $Directory
    if($Before.Count -ne $after.Count){throw 'Critical profile file count changed during delivery.'}
    foreach($name in $Before.Keys){if(-not $after.ContainsKey($name) -or $after[$name] -ne $Before[$name]){throw 'A critical profile file changed during delivery.'}}
}

function Resolve-EDReleaseCommit {
    param([Parameter(Mandatory)][string]$Tag,[scriptblock]$Request={param($url) Invoke-EDPublicDownload -URL $url})
    if($Tag -notmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'){throw 'Unsupported release tag.'}
    $reference=Get-EDPublicJSON "git/ref/tags/$Tag" $Request
    $object=$reference.object
    for($depth=0;$depth -lt 4;$depth++){
        if($object.type -eq 'commit' -and $object.sha -match '^[0-9a-f]{40}$'){return $object.sha}
        if($object.type -ne 'tag' -or $object.sha -notmatch '^[0-9a-f]{40}$'){throw 'Invalid release tag object.'}
        $object=(Get-EDPublicJSON ("git/tags/"+$object.sha) $Request).object
    }
    throw 'Release tag chain is too deep.'
}

function Test-EDReleaseTag {
    param([string]$Tag,[string]$ExpectedCommit,[scriptblock]$Request={param($url) Invoke-EDPublicDownload -URL $url})
    try{$actual=Resolve-EDReleaseCommit $Tag $Request}catch{if($_.Exception.Message -match 'HTTP 404\.$'){return $false};throw}
    if($actual -ne $ExpectedCommit){throw 'Existing release tag points to different source; increment app.Version.'}
    return $true
}

function Assert-EDCreatedReleaseTag {
    param($Reference,[string]$Tag,[string]$ExpectedCommit)
    if($Reference.ref -cne "refs/tags/$Tag" -or $Reference.object.type -cne 'commit' -or $Reference.object.sha -cne $ExpectedCommit){
        throw 'Tag creation did not acknowledge the expected release commit.'
    }
}

function Wait-EDReleaseTag {
    param([string]$Tag,[string]$ExpectedCommit,[scriptblock]$Request,[scriptblock]$Delay={param($seconds) Start-Sleep -Seconds $seconds})
    for($attempt=1;$attempt -le 5;$attempt++){
        if(Test-EDReleaseTag $Tag $ExpectedCommit $Request){return}
        if($attempt -lt 5){& $Delay $attempt}
    }
    throw 'Exact release tag did not become visible within five checks.'
}

function Save-EDReleaseAssets {
    param([Parameter(Mandatory)]$Release,[Parameter(Mandatory)][string]$ExpectedCommit,[Parameter(Mandatory)][string]$Directory,[scriptblock]$Download={param($url,$path,$limit) Invoke-EDPublicDownload -URL $url -Destination $path -MaxBytes $limit})
    $version=$Release.tag_name -replace '^v',''
    if($Release.draft -or $Release.prerelease -or $version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' -or $Release.tag_name -ne "v$version" -or $Release.html_url -ne "https://github.com/$script:EDDeliveryRepository/releases/tag/v$version"){throw 'Invalid stable release identity.'}
    $Directory=Get-EDSafePath $Directory
    [IO.Directory]::CreateDirectory($Directory)|Out-Null
    Save-EDPublicReleaseAsset $Release $version 'release-manifest.json' $Directory $Download
    $m=Get-Content -LiteralPath (Join-Path $Directory 'release-manifest.json') -Raw|ConvertFrom-Json
    if($m.sourceCommit -ne $ExpectedCommit -or $m.version -ne $version){throw 'Release manifest belongs to different source.'}
    if($m.schemaVersion -notin @(1,2,3)){throw 'Unsupported release manifest schema.'}
    $names=@(Get-EDPublicArtifactNames $version $m.schemaVersion)
    if($m.schemaVersion -ge 2 -and @($Release.assets | Where-Object { $_.name -notin ($names + 'release-manifest.json') }).Count){throw 'Unexpected public release asset.'}
    foreach($name in $names){
        Save-EDPublicReleaseAsset $Release $version $name $Directory $Download
    }
    return Test-EDReleaseDirectory $Directory $ExpectedCommit
}

function Save-EDPublicReleaseAsset {
    param($Release,[string]$Version,[string]$Name,[string]$Directory,[scriptblock]$Download)
    $matching=@($Release.assets|Where-Object name -EQ $Name)
    $limit=if($Name -match '\.(exe|zip)$'){268435456}else{8388608}
    if($matching.Count -ne 1){throw "Missing or duplicate GitHub release asset: $Name"}
    $asset=$matching[0]
    $expectedURL="https://github.com/$script:EDDeliveryRepository/releases/download/v$Version/$Name"
    if($asset.state -ne 'uploaded' -or $asset.browser_download_url -ne $expectedURL -or $asset.size -le 0 -or $asset.size -gt $limit -or $asset.digest -notmatch '^sha256:[0-9a-f]{64}$'){throw 'Invalid GitHub asset identity, size or digest.'}
    $path=Get-EDSafePath (Join-Path $Directory $Name)
    $cached=(Test-Path -LiteralPath $path -PathType Leaf) -and (Get-Item -LiteralPath $path).Length -eq $asset.size -and (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -eq $asset.digest.Substring(7)
    if(-not $cached){& $Download $expectedURL $path $limit}
    if(-not(Test-Path -LiteralPath $path -PathType Leaf) -or (Get-Item -LiteralPath $path).Length -ne $asset.size -or (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $asset.digest.Substring(7)){throw "Downloaded GitHub artifact verification failed: $Name"}
}

function Invoke-EDDeliveryChild {
    param([Parameter(Mandatory)][string]$File,[Parameter(Mandatory)][string[]]$Arguments,[int]$TimeoutSeconds=600)
    $info=[Diagnostics.ProcessStartInfo]::new();$info.FileName=$File;$info.WorkingDirectory=[IO.Path]::GetDirectoryName($File);$info.UseShellExecute=$false;$info.CreateNoWindow=$true;$info.WindowStyle=[Diagnostics.ProcessWindowStyle]::Hidden
    foreach($arg in $Arguments){$info.ArgumentList.Add($arg)}
    $process=[Diagnostics.Process]::Start($info)
    if(-not $process.WaitForExit($TimeoutSeconds*1000)){throw 'Delivery child is still running; no process was forcibly terminated.'}
    return $process.ExitCode
}

function Get-EDInstallationIdentity {
    param([string]$Target)
    $key='HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\EDCopilot.Desktop_is1'
    $registered='';$registeredVersion=''
    if(Test-Path -LiteralPath $key){
        $registration=Get-ItemProperty -LiteralPath $key
        $registered=(Get-EDSafePath $registration.InstallLocation).TrimEnd('\','/')
        $registeredVersion=$registration.DisplayVersion
        if(-not [string]::Equals($registered,$Target,[StringComparison]::OrdinalIgnoreCase)){throw 'Chosen directory differs from the registered installation.'}
    }
    $binary=Get-EDSafePath (Join-Path $Target 'ED Copilot.exe')
    $version='';$sha=''
    if(Test-Path -LiteralPath $binary){
        $version=(Get-Item -LiteralPath $binary).VersionInfo.ProductVersion
        if($version -notmatch '^\d+\.\d+\.\d+$'){throw 'Existing executable is not a recognized ED Copilot version.'}
        $sha=(Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
    }
    return [pscustomobject]@{version=$version;sha256=$sha;registeredDirectory=$registered;registeredVersion=$registeredVersion}
}

function Test-EDApplicationRunning {
    param([string]$Binary)
    return @(Get-CimInstance Win32_Process -Filter "Name='ED Copilot.exe'"|Where-Object {[string]::Equals($_.ExecutablePath,$Binary,[StringComparison]::OrdinalIgnoreCase)}).Count -gt 0
}

function Enter-EDDeliveryGate {
    # Match the native updater and normal app startup; hold through verification.
    $mutex=[Threading.Mutex]::new($false,'Local\EDCopilot.Updater.Apply')
    try{$owned=$mutex.WaitOne([TimeSpan]::FromSeconds(90))}catch [Threading.AbandonedMutexException]{$owned=$true}catch{$mutex.Dispose();throw}
    if(-not $owned){$mutex.Dispose();throw 'Another application update is still active.'}
    return {$mutex.ReleaseMutex();$mutex.Dispose()}.GetNewClosure()
}

function Install-EDVerifiedRelease {
    param([Parameter(Mandatory)]$Manifest,[Parameter(Mandatory)][string]$Directory,[Parameter(Mandatory)][string]$InstallDirectory,[Parameter(Mandatory)][string]$StateDirectory,[int]$WaitForExitMinutes=15,[switch]$AllowSameVersionRepair)
    if($env:OS -ne 'Windows_NT'){throw 'Installation requires Windows.'}
    $target=(Get-EDSafePath $InstallDirectory).TrimEnd('\','/')
    if([IO.Path]::GetPathRoot($target).TrimEnd('\','/') -eq $target){throw 'The drive root is not an application installation directory.'}
    $profile=Get-EDSafePath (Join-Path $env:LOCALAPPDATA 'ED Copilot')
    if((Test-EDPathOverlap $target $Directory) -or (Test-EDPathOverlap $target $StateDirectory) -or (Test-EDPathOverlap $target $profile) -or (Test-EDPathOverlap $StateDirectory $profile) -or (Test-EDPathOverlap $Directory $profile)){throw 'Installation, staging and player profile directories must be separate.'}
    $binary=Join-Path $target 'ED Copilot.exe'
    $expected=@($Manifest.artifacts|Where-Object name -EQ 'ED Copilot.exe')[0].sha256
    $identity=Get-EDInstallationIdentity $target
    $decision=Get-EDInstallDecision $identity.version $identity.sha256 $Manifest.version $expected -AllowSameVersionRepair:$AllowSameVersionRepair
    $currentResult=[ordered]@{installed=$false;alreadyCurrent=$true;version=$Manifest.version;commit=$Manifest.sourceCommit;executable=$binary;sha256=$expected}
    if($decision -eq 'current' -and $identity.registeredVersion -eq $Manifest.version){return $currentResult}
    $deadline=[DateTime]::UtcNow.AddMinutes($WaitForExitMinutes)
    $deferred=[ordered]@{installed=$false;deferred=$true;version=$Manifest.version;reason='Application remains open; verified update is staged for a later retry.'}
    do{
        if(-not(Test-EDApplicationRunning $binary)){break}
        if([DateTime]::UtcNow -ge $deadline){return $deferred}
        Start-Sleep -Seconds 5
    }while($true)
    $unlock=Enter-EDDeliveryGate
    try{
    if(Test-EDApplicationRunning $binary){return $deferred}
    # The native update helper may have won the gate while we were waiting.
    # Never use the pre-wait decision to replace a newer or foreign install.
    $identity=Get-EDInstallationIdentity $target
    $decision=Get-EDInstallDecision $identity.version $identity.sha256 $Manifest.version $expected -AllowSameVersionRepair:$AllowSameVersionRepair
    if($decision -eq 'current' -and $identity.registeredVersion -eq $Manifest.version){return $currentResult}
    # The player process has fully exited. Browser caches and updater state
    # are intentionally excluded; all commander data remains byte-identical.
    $before=Get-EDCriticalProfile $profile
    $backup=Get-EDSafePath (Join-Path $StateDirectory ('backups/'+[DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss')+'-'+[guid]::NewGuid().ToString('N')))
    [IO.Directory]::CreateDirectory($backup)|Out-Null
    Write-EDDeliveryJSON (Join-Path $backup 'profile-sha256.json') $before
    foreach($name in $before.Keys){$dest=Join-Path $backup ('profile/'+$name);[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($dest))|Out-Null;Copy-Item -LiteralPath (Join-Path $profile $name) -Destination $dest}
    if(Test-Path -LiteralPath $target){
        if(@(Get-ChildItem -LiteralPath $target -Recurse -Force|Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}).Count){throw 'Application backup refuses linked files.'}
        Copy-Item -LiteralPath $target -Destination (Join-Path $backup 'application') -Recurse
    }
    $setup=Get-EDSafePath (Join-Path $Directory "ED-Copilot-$($Manifest.version)-Setup.exe")
    $setupEntry=@($Manifest.artifacts|Where-Object name -EQ ([IO.Path]::GetFileName($setup)))[0]
    if((Get-FileHash -LiteralPath $setup -Algorithm SHA256).Hash.ToLowerInvariant() -ne $setupEntry.sha256){throw 'Staged setup changed before installation.'}
    $code=Invoke-EDDeliveryChild $setup @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/SP-','/NOCLOSEAPPLICATIONS','/NORESTARTAPPLICATIONS','/RESTARTEXITCODE=3010',('/DIR='+$target),('/LOG='+(Join-Path $backup 'install.log')))
    if($code -notin @(0,3010)){throw "Quiet installer failed with code $code; backup retained at $backup"}
    $registration=Get-EDInstallationIdentity $target
    if($registration.version -ne $Manifest.version -or $registration.sha256 -ne $expected){throw 'Installed executable identity differs from the verified release.'}
    if($registration.registeredVersion -ne $Manifest.version -or -not $registration.registeredDirectory){throw 'Installed application registration is inconsistent.'}
    Assert-EDCriticalProfile $before $profile
    $smoke=Join-Path $backup 'installed-headless.json'
    if((Invoke-EDDeliveryChild $binary @('--smoke-headless','--smoke-report',$smoke) -TimeoutSeconds 60) -ne 0){throw 'Installed isolated smoke failed.'}
    Assert-EDSmokeReport (Get-Content -LiteralPath $smoke -Raw|ConvertFrom-Json) $Manifest.version 'headless'
    Assert-EDCriticalProfile $before $profile
    return [ordered]@{installed=$true;alreadyCurrent=$false;version=$Manifest.version;commit=$Manifest.sourceCommit;executable=$binary;sha256=$expected;profileFilesPreserved=$before.Count;backup=$backup;headless=$true;rebootRequested=$code -eq 3010}
    }finally{& $unlock}
}
