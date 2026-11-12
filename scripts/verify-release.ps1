#requires -Version 7.2
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Directory,[Parameter(Mandatory)][string]$ExpectedCommit,[string]$SourceRoot='')
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'delivery-lib.ps1')
$manifest=Test-EDReleaseDirectory -Directory $Directory -ExpectedCommit $ExpectedCommit -SourceRoot $SourceRoot -RequireBuildEvidence
[ordered]@{ok=$true;version=$manifest.version;commit=$manifest.sourceCommit;sourceTreeSHA256=$manifest.sourceTreeSHA256;artifacts=$manifest.artifacts.Count}|ConvertTo-Json -Compress
