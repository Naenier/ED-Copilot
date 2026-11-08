#requires -Version 7.2
[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'delivery-lib.ps1')
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
    $null=New-EDReleaseManifest -Root $source -Directory $artifacts -Commit $commit -TestsPassed $true
}
try{
    Refresh-Fixture
    Check 'Complete package/source/ZIP/smokes validate' {$m=Test-EDReleaseDirectory $artifacts $commit $source;Must ($m.version -eq '2.3.4') 'Wrong version.'}
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
    foreach($file in Get-ChildItem -LiteralPath $artifacts -File|Where-Object Name -NE 'ED Copilot.exe'){$release.assets+= [pscustomobject]@{name=$file.Name;state='uploaded';size=$file.Length;digest='sha256:'+(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant();browser_download_url='https://github.com/Naenier/ED-Copilot/releases/download/v2.3.4/'+$file.Name}}
    $calls=@{n=0}
    $mock={param($url,$path,$limit) $calls['n']++;$name=[IO.Path]::GetFileName(([uri]$url).AbsolutePath);[IO.File]::WriteAllBytes($path,[IO.File]::ReadAllBytes((Join-Path $artifacts $name)))}
    $downloaded=Join-Path $temp 'downloaded'
    Check 'Mock public download verifies eight assets' {$null=Save-EDReleaseAssets $release $commit $downloaded $mock;Must ($calls.n -eq 8) 'Some release assets were not verified.'}
    Check 'Verified cache avoids repeated downloads' {$null=Save-EDReleaseAssets $release $commit $downloaded $mock;Must ($calls.n -eq 8) 'Identical assets downloaded twice.'}
    Check 'GitHub checksum disagreement rejected' {$old=$release.assets[0].digest;$release.assets[0].digest='sha256:'+('0'*64);Reject {Save-EDReleaseAssets $release $commit $downloaded $mock} 'verification failed';$release.assets[0].digest=$old}
    Check 'Mock API resolves exact tag commit' {$request={param($url) return ,[Text.Encoding]::UTF8.GetBytes('{"object":{"type":"commit","sha":"'+$commit+'"}}')};Must ((Resolve-EDReleaseCommit 'v2.3.4' $request) -eq $commit) 'Tag identity lost.'}
    Check 'Orphan tag for different commit blocks publication' {$request={param($url) return ,[Text.Encoding]::UTF8.GetBytes('{"object":{"type":"commit","sha":"'+('b'*40)+'"}}')};Reject {Test-EDReleaseTag 'v2.3.4' $commit $request} 'different source'}
    Check 'Missing tag can be created without suppressing unrelated errors' {$missing={param($url) throw 'Public GitHub download returned HTTP 404.'};Must (-not(Test-EDReleaseTag 'v2.3.4' $commit $missing)) 'Missing tag not recognized.';Reject {Test-EDReleaseTag 'v2.3.4' $commit {throw 'Public GitHub download returned HTTP 403.'}} '403'}
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
