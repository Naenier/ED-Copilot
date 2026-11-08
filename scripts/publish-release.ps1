#requires -Version 7.2
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Directory,[Parameter(Mandatory)][string]$ExpectedCommit,[string]$SourceRoot=(Join-Path $PSScriptRoot '..'))
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'delivery-lib.ps1')
$manifest=Test-EDReleaseDirectory -Directory $Directory -ExpectedCommit $ExpectedCommit -SourceRoot $SourceRoot
$head=Get-EDPublicJSON 'git/ref/heads/main'
if($head.object.sha -ne $ExpectedCommit){Write-Output 'A newer source commit superseded this build; publication skipped.';return}
$version=$manifest.version
$notes=Join-Path $SourceRoot "docs/RELEASE-$version.md"
if(-not(Test-Path -LiteralPath $notes -PathType Leaf)){throw 'Release notes for this version are missing.'}
$assets=@($manifest.artifacts|Where-Object published|ForEach-Object {Join-Path $Directory $_.name})
$assets+=Join-Path $Directory 'release-manifest.json'
$tagExists=Test-EDReleaseTag "v$version" $ExpectedCommit
$existing=$null
try{$existing=Get-EDPublicJSON "releases/tags/v$version"}catch{if($_.Exception.Message -notmatch 'HTTP 404\.$'){throw}}
if($null -ne $existing){
    if($existing.draft -or $existing.prerelease -or -not $tagExists){throw 'Existing release version has invalid provenance; increment app.Version.'}
    foreach($file in $assets){
        $name=[IO.Path]::GetFileName($file)
        $remote=@($existing.assets|Where-Object name -EQ $name)
        if($remote.Count -ne 1 -or $remote[0].digest -ne ('sha256:'+(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant())){throw 'Existing immutable release has different artifacts; increment app.Version.'}
    }
    Write-Output "Verified release v$version is already published."
    return
}
# A pre-existing orphan tag must never silently bind this package to other
# source. Create an exact tag explicitly, then require it for publication.
if((Get-EDPublicJSON 'git/ref/heads/main').object.sha -ne $ExpectedCommit){Write-Output 'A newer source commit superseded this build; publication skipped.';return}
if(-not $tagExists){
    & gh api --method POST "repos/$script:EDDeliveryRepository/git/refs" -f "ref=refs/tags/v$version" -f "sha=$ExpectedCommit" --silent
    if($LASTEXITCODE -ne 0){throw 'Could not create the exact release tag.'}
}
if(-not(Test-EDReleaseTag "v$version" $ExpectedCommit)){throw 'Exact release tag is not available.'}
if((Get-EDPublicJSON 'git/ref/heads/main').object.sha -ne $ExpectedCommit){Write-Output 'A newer source commit superseded this build; publication skipped.';return}
# gh uses the workflow's short-lived token; credentials are never embedded.
& gh release create "v$version" @assets --verify-tag --title "ED Copilot $version" --notes-file $notes --repo $script:EDDeliveryRepository
if($LASTEXITCODE -ne 0){throw 'GitHub release publication failed.'}
