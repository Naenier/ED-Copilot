#requires -Version 7.2
[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'delivery-lib.ps1')
. (Join-Path $PSScriptRoot 'release-sbom.ps1')
$count=0
function Check([string]$Name,[scriptblock]$Body){& $Body;$script:count++;Write-Verbose "PASS $Name"}
function Must([bool]$Condition,[string]$Message){if(-not $Condition){throw $Message}}
function Reject([scriptblock]$Body,[string]$Pattern){$caught=$false;try{& $Body|Out-Null}catch{$caught=$true;if($Pattern -and $_.Exception.Message -notmatch $Pattern){throw}};if(-not $caught){throw 'Expected validation to reject the fixture.'}}
$temp=Join-Path ([IO.Path]::GetTempPath()) ('ed-copilot-delivery-test-'+[guid]::NewGuid().ToString('N'))
$source=Join-Path $temp 'source';$artifacts=Join-Path $temp 'artifacts'
[IO.Directory]::CreateDirectory((Join-Path $source 'internal/app'))|Out-Null
[IO.Directory]::CreateDirectory($artifacts)|Out-Null
$commit='a'*40
[IO.File]::WriteAllText((Join-Path $source 'internal/app/engine.go'),'package app; const Version = "2.3.4"')
[IO.File]::WriteAllText((Join-Path $source 'README.md'),'')
$binary=[Text.Encoding]::UTF8.GetBytes('synthetic executable')
[IO.File]::WriteAllBytes((Join-Path $artifacts 'ED Copilot.exe'),$binary)
[IO.File]::WriteAllBytes((Join-Path $artifacts 'ED-Copilot-2.3.4-Setup.exe'),[Text.Encoding]::UTF8.GetBytes('synthetic setup'))
foreach($kind in @('headless','native','stress','installer')){Write-EDDeliveryJSON (Join-Path $artifacts "smoke-$kind.json") @{ok=$true;version='2.3.4';networkDisabled=$true;profileUnchanged=$true;webviewGeometry=@{ok=$true}}}
$zipPath=Join-Path $artifacts 'ED-Copilot-2.3.4-windows-amd64.zip'
$zip=[IO.Compression.ZipFile]::Open($zipPath,[IO.Compression.ZipArchiveMode]::Create)
try{$entry=$zip.CreateEntry('ED Copilot.exe');$stream=$entry.Open();try{$stream.Write($binary,0,$binary.Length)}finally{$stream.Dispose()}}finally{$zip.Dispose()}
function Refresh-Fixture {
    $lines=@('ED Copilot.exe','ED-Copilot-2.3.4-Setup.exe','ED-Copilot-2.3.4-windows-amd64.zip')|ForEach-Object{(Get-FileHash -LiteralPath (Join-Path $artifacts $_) -Algorithm SHA256).Hash.ToLowerInvariant()+'  '+$_}
    [IO.File]::WriteAllLines((Join-Path $artifacts 'SHA256SUMS.txt'),$lines)
    $buildInfo = @{
        Path='github.com/Naenier/ED-Copilot/cmd/copilot'; GoVersion='go1.27.3'
        Settings=@(@{Key='GOOS';Value='windows'},@{Key='GOARCH';Value='amd64'},@{Key='CGO_ENABLED';Value='0'})
        Deps=@(@{Path='github.com/fixture/module';Version='v1.2.3';Sum=('h1:'+('A'*43)+'=')})
    }
    $application = New-EDApplicationSBOM (Join-Path $artifacts 'ED Copilot.exe') '2.3.4' $buildInfo
    $setup = New-EDSBOMFileComponent (Join-Path $artifacts 'ED-Copilot-2.3.4-Setup.exe') 'ED Copilot Setup' '2.3.4' 'ed-copilot-setup'
    Write-EDDeliveryJSON (Join-Path $artifacts 'application.cdx.json') $application
    Write-EDDeliveryJSON (Join-Path $artifacts 'installer.cdx.json') @{bomFormat='CycloneDX';specVersion='1.6';serialNumber=('urn:uuid:'+[Guid]::NewGuid().ToString('D'));metadata=@{component=$setup};components=@($application.metadata.component)}
    $null=New-EDReleaseManifest -Root $source -Directory $artifacts -Commit $commit -TestsPassed $true
}
try{
    Refresh-Fixture
    Check 'Complete package/source/ZIP/smokes and SBOMs validate' {$m=Test-EDReleaseDirectory $artifacts $commit $source -RequireBuildEvidence;Must ($m.version -eq '2.3.4' -and $m.schemaVersion -eq 3) 'Wrong version or schema.';Must (@($m.artifacts | Where-Object published).Count -eq 3) 'Release exposes internal evidence.'}
    Check 'Inventory identifies the actual Go toolchain and compiled module' {
        $bom=Get-Content -LiteralPath (Join-Path $artifacts 'application.cdx.json') -Raw | ConvertFrom-Json
        Must ($bom.components[0].version -eq '1.27.3') 'Missing Go standard library version.'
        Must ($bom.components[1].purl -eq 'pkg:golang/github.com/fixture/module@v1.2.3') 'Compiled Go module missing.'
        Must ($bom.components[1].properties[0].value -eq ('h1:'+('A'*43)+'=')) 'Module checksum missing.'
    }
    Check 'Attestation inventories require a CycloneDX serial number' {
        $path=Join-Path $artifacts 'application.cdx.json'
        try {
            $bom=Get-Content -LiteralPath $path -Raw | ConvertFrom-Json -AsHashtable
            $bom.Remove('serialNumber')
            Write-EDDeliveryJSON $path $bom
            $null=New-EDReleaseManifest $source $artifacts $commit $true
            Reject {Test-EDReleaseDirectory $artifacts $commit} 'Invalid SBOM serial number'
        } finally { Refresh-Fixture }
    }
    Check 'An inventory with a mismatched executable digest is rejected' {
        $path=Join-Path $artifacts 'application.cdx.json'
        try {
            $bom=Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
            $bom.metadata.component.hashes[0].content='0'*64
            Write-EDDeliveryJSON $path $bom
            $null=New-EDReleaseManifest $source $artifacts $commit $true
            Reject {Test-EDReleaseDirectory $artifacts $commit} 'SBOM component'
        } finally { Refresh-Fixture }
    }
    Check 'Release staging requires both inventories' {
        $path=Join-Path $artifacts 'installer.cdx.json'
        $bytes=[IO.File]::ReadAllBytes($path)
        try {
            [IO.File]::Delete($path)
            Reject {Test-EDReleaseDirectory $artifacts $commit -RequireBuildEvidence} 'Missing release artifact: installer.cdx.json'
        } finally { [IO.File]::WriteAllBytes($path,$bytes) }
    }
    Check 'Publication verifies provenance for every asset and SBOMs for both packages' {
        $m=Test-EDReleaseDirectory $artifacts $commit -RequireBuildEvidence
        $verified=[Collections.Generic.List[string]]::new()
        $checkAttestation={
            param($path,$predicate,$sourceDigest)
            Must ($sourceDigest -eq $commit) 'Verification did not pin the checked source commit.'
            $verified.Add(([IO.Path]::GetFileName($path)+': '+$predicate))
        }
        Assert-EDReleaseAttestations $artifacts $m $checkAttestation
        Must ($verified.Count -eq 6) 'Unexpected number of attestation verifications.'
        Must ($verified.Contains('release-manifest.json: https://slsa.dev/provenance/v1')) 'Manifest provenance verification missing.'
        Must ($verified.Contains('ED-Copilot-2.3.4-Setup.exe: https://cyclonedx.org/bom') -and $verified.Contains('ED-Copilot-2.3.4-windows-amd64.zip: https://cyclonedx.org/bom')) 'Package inventory attestation verification missing.'
        Reject {Assert-EDReleaseAttestations $artifacts $m {throw 'Fixture attestation invalid.'}} 'attestation invalid'
    }
    Check 'Different source commit rejected' {Reject {Test-EDReleaseDirectory $artifacts ('b'*40)} 'provenance'}
    Check 'Modified source rejected' {[IO.File]::WriteAllText((Join-Path $source 'README.md'),'modified');Reject {Test-EDReleaseDirectory $artifacts $commit $source} 'source';[IO.File]::WriteAllText((Join-Path $source 'README.md'),'')}
    Check 'Tampered setup rejected' {$p=Join-Path $artifacts 'ED-Copilot-2.3.4-Setup.exe';$old=[IO.File]::ReadAllBytes($p);[IO.File]::WriteAllText($p,'tampered');Reject {Test-EDReleaseDirectory $artifacts $commit} 'mismatch';[IO.File]::WriteAllBytes($p,$old)}
    Check 'Manifest traversal rejected before path access' {$p=Join-Path $artifacts 'release-manifest.json';$old=[IO.File]::ReadAllText($p);$m=$old|ConvertFrom-Json;$m.artifacts[0].name='../elsewhere.exe';Write-EDDeliveryJSON $p $m;Reject {Test-EDReleaseDirectory $artifacts $commit} 'Invalid';[IO.File]::WriteAllText($p,$old)}
    Check 'Failed smoke cannot produce a release manifest' {$p=Join-Path $artifacts 'smoke-native.json';$old=[IO.File]::ReadAllText($p);$r=$old|ConvertFrom-Json;$r.ok=$false;Write-EDDeliveryJSON $p $r;Reject {New-EDReleaseManifest $source $artifacts $commit $true} 'smoke';[IO.File]::WriteAllText($p,$old)}
    Check 'Truthy strings cannot stand in for smoke success' {Reject {Assert-EDSmokeReport @{ok='false';networkDisabled='false';version='2.3.4'} '2.3.4' 'headless'} 'smoke'}
    Check 'Skipped tests cannot validate for publication' {$p=Join-Path $artifacts 'release-manifest.json';$old=[IO.File]::ReadAllText($p);$m=$old|ConvertFrom-Json;$m.testsPassed=$false;Write-EDDeliveryJSON $p $m;Reject {Test-EDReleaseDirectory $artifacts $commit} 'provenance';[IO.File]::WriteAllText($p,$old)}
    Check 'Checksums cannot disagree with artifact manifest' {$p=Join-Path $artifacts 'SHA256SUMS.txt';$old=[IO.File]::ReadAllText($p);[IO.File]::WriteAllText($p,$old.Replace($old.Substring(0,64),('0'*64)));$null=New-EDReleaseManifest $source $artifacts $commit $true;Reject {Test-EDReleaseDirectory $artifacts $commit} 'checksum sources';[IO.File]::WriteAllText($p,$old);Refresh-Fixture}
    Check 'Portable executable stream identity is checked' {$old=[IO.File]::ReadAllBytes($zipPath);[IO.File]::Delete($zipPath);$z=[IO.Compression.ZipFile]::Open($zipPath,[IO.Compression.ZipArchiveMode]::Create);try{$e=$z.CreateEntry('ED Copilot.exe');$s=$e.Open();try{$s.Write([byte[]]::new($binary.Length),0,$binary.Length)}finally{$s.Dispose()}}finally{$z.Dispose()};Refresh-Fixture;Reject {Test-EDReleaseDirectory $artifacts $commit} 'Portable executable identity';[IO.File]::WriteAllBytes($zipPath,$old);Refresh-Fixture}
    Check 'Confirmed same executable is idempotent' {Must ((Get-EDInstallDecision '2.3.4' ('a'*64) '2.3.4' ('a'*64)) -eq 'current') 'Same release would reinstall.'}
    Check 'Same version different bytes rejected by default' {Reject {Get-EDInstallDecision '2.3.4' ('a'*64) '2.3.4' ('b'*64)} 'Same version'}
    Check 'Explicit verified repair is possible' {Must ((Get-EDInstallDecision '2.3.4' ('a'*64) '2.3.4' ('b'*64) -AllowSameVersionRepair) -eq 'repair') 'Repair not available.'}
    Check 'Downgrade rejected' {Reject {Get-EDInstallDecision '2.4.0' ('a'*64) '2.3.4' ('b'*64)} 'downgrade'}
    Check 'Newer version upgrades' {Must ((Get-EDInstallDecision '2.3.3' ('a'*64) '2.3.4' ('b'*64)) -eq 'upgrade') 'Upgrade lost.'}
    Check 'Foreign and plaintext download hosts rejected' {foreach($u in @('http://github.com/Naenier/ED-Copilot/releases/download/v2.3.4/x','https://evil.example/x','https://github.com/another/project/releases/download/v2.3.4/x','https://user@github.com/Naenier/ED-Copilot/releases/download/v2.3.4/x')){Reject {Assert-EDPublicURL ([uri]$u)} 'Untrusted'}}
    Check 'Approved public release redirect allowed' {Assert-EDPublicURL ([uri]'https://release-assets.githubusercontent.com/fixture')}
    $release=[pscustomobject]@{tag_name='v2.3.4';html_url='https://github.com/Naenier/ED-Copilot/releases/tag/v2.3.4';draft=$false;prerelease=$false;assets=@()}
    function Get-FixtureAssets($Directory, $Names) {
        foreach($name in $Names){
            $file=Get-Item -LiteralPath (Join-Path $Directory $name)
            [pscustomobject]@{name=$file.Name;state='uploaded';size=$file.Length;digest='sha256:'+(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant();browser_download_url='https://github.com/Naenier/ED-Copilot/releases/download/v2.3.4/'+$file.Name}
        }
    }
    $release.assets=@(Get-FixtureAssets $artifacts (@('release-manifest.json') + @(Get-EDPublicArtifactNames '2.3.4' 2)))
    $calls=@{n=0}
    $mock={param($url,$path,$limit) $calls['n']++;$name=[IO.Path]::GetFileName(([uri]$url).AbsolutePath);[IO.File]::WriteAllBytes($path,[IO.File]::ReadAllBytes((Join-Path $artifacts $name)))}
    $downloaded=Join-Path $temp 'downloaded'
    Check 'Public download verifies four assets without downloading internal evidence' {
        $null=Save-EDReleaseAssets $release $commit $downloaded $mock
        Must ($calls.n -eq 4 -and @(Get-ChildItem -LiteralPath $downloaded -Filter 'smoke-*.json').Count -eq 0 -and @(Get-ChildItem -LiteralPath $downloaded -Filter '*.cdx.json').Count -eq 0) 'Public delivery fetched internal evidence or missed a public asset.'
    }
    Check 'CI verification requires internal reports' {Reject {Test-EDReleaseDirectory $downloaded $commit -RequireBuildEvidence} 'Missing release artifact: smoke-'}
    Check 'Public ZIP cannot embed an internal report' {
        $old=[IO.File]::ReadAllBytes($zipPath)
        try {
            $z=[IO.Compression.ZipFile]::Open($zipPath,[IO.Compression.ZipArchiveMode]::Update)
            try { $null=$z.CreateEntry('diagnostics/smoke-native.json') } finally { $z.Dispose() }
            Refresh-Fixture
            Reject {Test-EDReleaseDirectory $artifacts $commit} 'includes CI-only evidence'
        } finally { [IO.File]::WriteAllBytes($zipPath,$old);Refresh-Fixture }
    }
    Check 'Current releases reject internal evidence attached publicly' {
        $extra=$release.PSObject.Copy()
        $extra.assets=@($release.assets)+@(Get-FixtureAssets $artifacts @('smoke-native.json'))
        Reject {Save-EDReleaseAssets $extra $commit $downloaded $mock} 'Unexpected public release asset'
    }
    Check 'Verified cache avoids repeated downloads' {$null=Save-EDReleaseAssets $release $commit $downloaded $mock;Must ($calls.n -eq 4) 'Identical assets downloaded twice.'}
    Check 'Legacy release still downloads and verifies all eight assets' {
        $legacy=Join-Path $temp 'legacy';[IO.Directory]::CreateDirectory($legacy)|Out-Null
        foreach($file in Get-ChildItem -LiteralPath $artifacts -File){Copy-Item -LiteralPath $file.FullName -Destination $legacy}
        $manifestPath=Join-Path $legacy 'release-manifest.json'
        $legacyManifest=Get-Content -LiteralPath $manifestPath -Raw|ConvertFrom-Json
        $legacyManifest.schemaVersion=1
        $legacyManifest.artifacts=@($legacyManifest.artifacts | Where-Object name -NotIn $script:EDReleaseSBOMNames)
        foreach($entry in $legacyManifest.artifacts){$entry.published=$entry.name -ne 'ED Copilot.exe'}
        Write-EDDeliveryJSON $manifestPath $legacyManifest
        $legacyRelease=$release.PSObject.Copy()
        $legacyRelease.assets=@(Get-FixtureAssets $legacy (@('release-manifest.json')+@(Get-EDPublicArtifactNames '2.3.4' 1)))
        $legacyCalls=@{n=0}
        $downloadLegacy={param($url,$path,$limit) $legacyCalls.n++;$name=[IO.Path]::GetFileName(([uri]$url).AbsolutePath);Copy-Item -LiteralPath (Join-Path $legacy $name) -Destination $path}
        $m=Save-EDReleaseAssets $legacyRelease $commit (Join-Path $temp 'legacy-download') $downloadLegacy
        Must ($m.schemaVersion -eq 1 -and $legacyCalls.n -eq 8) 'Legacy release verification no longer works.'
    }
    Check 'GitHub checksum disagreement rejected' {$old=$release.assets[0].digest;$release.assets[0].digest='sha256:'+('0'*64);Reject {Save-EDReleaseAssets $release $commit $downloaded $mock} 'verification failed';$release.assets[0].digest=$old}
    Check 'Mock API resolves exact tag commit' {$request={param($url) return ,[Text.Encoding]::UTF8.GetBytes('{"object":{"type":"commit","sha":"'+$commit+'"}}')};Must ((Resolve-EDReleaseCommit 'v2.3.4' $request) -eq $commit) 'Tag identity lost.'}
    Check 'Orphan tag for different commit blocks publication' {$request={param($url) return ,[Text.Encoding]::UTF8.GetBytes('{"object":{"type":"commit","sha":"'+('b'*40)+'"}}')};Reject {Test-EDReleaseTag 'v2.3.4' $commit $request} 'different source'}
    Check 'Missing tag can be created without suppressing unrelated errors' {$missing={param($url) throw 'Public GitHub download returned HTTP 404.'};Must (-not(Test-EDReleaseTag 'v2.3.4' $commit $missing)) 'Missing tag not recognized.';Reject {Test-EDReleaseTag 'v2.3.4' $commit {throw 'Public GitHub download returned HTTP 403.'}} '403'}
    Check 'Publisher metadata cannot send credentials to a CDN' {
        Reject {Invoke-EDPublicDownload -URL 'https://release-assets.githubusercontent.com/fixture' -PublisherMetadata} 'restricted to GitHub API'
    }
    Check 'Tag creation confirms exact name and commit' {
        Assert-EDCreatedReleaseTag @{ref='refs/tags/v2.3.4';object=@{type='commit';sha=$commit}} 'v2.3.4' $commit
        Reject {Assert-EDCreatedReleaseTag @{ref='refs/tags/v2.3.4';object=@{type='commit';sha=('b'*40)}} 'v2.3.4' $commit} 'expected release commit'
    }
    Check 'New exact tag can appear after an initial missing response' {
        $observed=@{reads=0;delays=0}
        $request={param($url) $observed.reads++;if($observed.reads -eq 1){throw 'GitHub download returned HTTP 404.'};return ,[Text.Encoding]::UTF8.GetBytes('{"object":{"type":"commit","sha":"'+$commit+'"}}')}
        Wait-EDReleaseTag 'v2.3.4' $commit $request {param($seconds) $observed.delays+=$seconds}
        Must ($observed.reads -eq 2 -and $observed.delays -eq 1) 'Delayed tag was not verified after a single retry.'
    }
    Check 'Missing tag retries are bounded and other failures stop immediately' {
        $observed=@{reads=0;delays=0}
        $request={param($url) $observed.reads++;throw 'GitHub download returned HTTP 404.'}
        Reject {Wait-EDReleaseTag 'v2.3.4' $commit $request {param($seconds) $observed.delays+=$seconds}} 'five checks'
        Must ($observed.reads -eq 5 -and $observed.delays -eq 10) 'Tag visibility wait was not bounded.'
        Reject {Wait-EDReleaseTag 'v2.3.4' $commit {throw 'GitHub download returned HTTP 403.'} {throw 'Unexpected retry'}} '403'
    }
    Check 'Prerelease cannot be installed as stable' {$release.prerelease=$true;Reject {Save-EDReleaseAssets $release $commit $downloaded $mock} 'stable release';$release.prerelease=$false}
    $profile=Join-Path $temp 'profile';[IO.Directory]::CreateDirectory((Join-Path $profile 'WebView2'))|Out-Null
    [IO.File]::WriteAllText((Join-Path $profile 'settings-go-v2.json'),'synthetic commander data')
    [IO.File]::WriteAllText((Join-Path $profile 'WebView2/volatile'),'cache')
    $before=Get-EDCriticalProfile $profile
    Check 'Volatile browser cache excluded from critical checkpoint' {Must ($before.Count -eq 1) 'Browser cache was treated as commander data.';[IO.File]::WriteAllText((Join-Path $profile 'WebView2/volatile'),'changed');Assert-EDCriticalProfile $before $profile}
    Check 'Critical profile mutation rejected' {[IO.File]::WriteAllText((Join-Path $profile 'settings-go-v2.json'),'changed');Reject {Assert-EDCriticalProfile $before $profile} 'critical profile'}
    Check 'Installation cannot back itself up recursively' {$m=Test-EDReleaseDirectory $artifacts $commit;Reject {Install-EDVerifiedRelease $m $artifacts (Join-Path $temp 'install') (Join-Path $temp 'install/state') -WaitForExitMinutes 0} 'must be separate'}
    Check 'Recheck after wait prevents downgrade if another updater won' {
        $m=Test-EDReleaseDirectory $artifacts $commit
        $observed=@{reads=0;installCalls=0}
        function Get-EDInstallationIdentity($Target){$observed.reads++;$v=if($observed.reads -eq 1){'2.3.3'}else{'2.3.5'};return [pscustomobject]@{version=$v;sha256=('0'*64);registeredVersion=$v;registeredDirectory=$Target}}
        function Test-EDApplicationRunning($Binary){return $false}
        function Enter-EDDeliveryGate {return {}}
        function Invoke-EDDeliveryChild {$observed.installCalls++;throw 'Installer must never run for a downgrade.'}
        Reject {Install-EDVerifiedRelease $m $artifacts (Join-Path $temp 'install') (Join-Path $temp 'state') -WaitForExitMinutes 0} 'downgrade'
        Must ($observed.reads -eq 2 -and $observed.installCalls -eq 0) 'Fresh installed identity was not checked under the gate.'
    }
    Check 'Deterministic source hashes include empty Git blob correctly' {$a=Get-EDSourceState $source;$b=Get-EDSourceState $source;Must ($a.treeSHA256 -eq $b.treeSHA256) 'Unstable source digest.';Must (@($a.files|Where-Object path -EQ 'README.md')[0].sha -eq 'e69de29bb2d1d6434b8b29ae775ad8c2e48c5391') 'Empty Git blob digest differs.'}
    Check 'Source tree normalizes checkout line endings but blob hashes stay exact' {$p=Join-Path $source 'README.md';[IO.File]::WriteAllText($p,"one`r`ntwo`r`n");$a=Get-EDSourceState $source;[IO.File]::WriteAllText($p,"one`ntwo`n");$b=Get-EDSourceState $source;Must ($a.treeSHA256 -eq $b.treeSHA256) 'Checkout EOL changed canonical source identity.';Must (@($a.files|Where-Object path -EQ 'README.md')[0].sha -ne @($b.files|Where-Object path -EQ 'README.md')[0].sha) 'Raw uploaded blob identity was normalized.';[IO.File]::WriteAllText($p,'')}
    Check 'Connector preparation includes new and empty blobs' {$previous=Join-Path $temp 'previous.json';Write-EDDeliveryJSON $previous @{tree=('b'*40);commit=('c'*40);files=@()};$out=Join-Path $temp 'publication';& (Join-Path $PSScriptRoot 'prepare-source.ps1') -SourceRoot $source -PreviousManifest $previous -OutputDirectory $out|Out-Null;$prepared=Get-Content -LiteralPath (Join-Path $out 'manifest.json') -Raw|ConvertFrom-Json;Must ($prepared.changed.Count -eq 2 -and $prepared.chunks -eq 2) 'Connector publication omitted new or empty blobs.'}
    Check 'Delivery scripts parse' {foreach($path in @((Join-Path $PSScriptRoot '../build.ps1'))+@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*.ps1' -File|ForEach-Object FullName)){$errors=$null;$tokens=$null;$null=[Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$errors);Must ($errors.Count -eq 0) ('PowerShell syntax error: '+$path)}}
    Write-Output "Delivery contract tests: $count passed (synthetic data; no network or real installation)."
}finally{
    $checked=[IO.Path]::GetFullPath($temp)
    $tempRoot=[IO.Path]::GetFullPath([IO.Path]::GetTempPath())
    if(-not $checked.StartsWith($tempRoot,[StringComparison]::OrdinalIgnoreCase) -or -not [IO.Path]::GetFileName($checked).StartsWith('ed-copilot-delivery-test-')){throw 'Unexpected disposable test directory.'}
    Remove-Item -LiteralPath $checked -Recurse -Force
}
