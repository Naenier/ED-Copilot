#requires -Version 7.2
[CmdletBinding()]
param(
    [string]$SourceRoot=(Join-Path $PSScriptRoot '..'),
    [Parameter(Mandatory)][string]$PreviousManifest,
    [Parameter(Mandatory)][string]$OutputDirectory
)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'delivery-lib.ps1')
$root=(Resolve-Path -LiteralPath $SourceRoot).ProviderPath
$out=[IO.Path]::GetFullPath($OutputDirectory)
$old=Get-Content -LiteralPath $PreviousManifest -Raw | ConvertFrom-Json
if($old.tree -notmatch '^[0-9a-f]{40}$' -or $old.commit -notmatch '^[0-9a-f]{40}$'){throw 'Previous publication identity is missing.'}
$previous=@{}
foreach($file in $old.files){$previous[$file.path]=$file}
$source=Get-EDSourceState $root
$changed=@($source.files | Where-Object {-not $previous.ContainsKey($_.path) -or $previous[$_.path].sha -ne $_.sha})
$current=@{}
foreach($file in $source.files){$current[$file.path]=$true}
$removed=@($old.files | Where-Object {-not $current.ContainsKey($_.path)} | ForEach-Object path)
[IO.Directory]::CreateDirectory($out)|Out-Null
$chunkCount=0
foreach($file in $changed){
    $encoded=[Convert]::ToBase64String([IO.File]::ReadAllBytes((Join-Path $root $file.path)))
    $part=0
    # Empty files still need a Git blob; base64 chunks remain four-aligned.
    do {
        $start=$part*32000
        $length=[Math]::Min(32000,$encoded.Length-$start)
        Write-EDDeliveryJSON (Join-Path $out "$chunkCount.json") ([ordered]@{path=$file.path;part=$part;content=$encoded.Substring($start,$length)})
        $part++;$chunkCount++
    } while($part*32000 -lt $encoded.Length)
}
$manifest=[ordered]@{version=(Get-EDCopilotVersion $root);baseTree=$old.tree;parentCommit=$old.commit;sourceTreeSHA256=$source.treeSHA256;files=$source.files;changed=$changed;removed=$removed;chunks=$chunkCount}
Write-EDDeliveryJSON (Join-Path $out 'manifest.json') $manifest
[ordered]@{status='prepared';version=$manifest.version;manifest=(Join-Path $out 'manifest.json');files=$source.files.Count;changed=$changed.Count;removed=$removed.Count;chunks=$chunkCount}|ConvertTo-Json -Compress
