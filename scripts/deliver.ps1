#requires -Version 7.2
<#
.SYNOPSIS
Wait for an exact verified release and optionally install it without manual checks.
.EXAMPLE
./scripts/deliver.ps1 -Release -Install -ExpectedCommit <full GitHub commit SHA>
.EXAMPLE
./scripts/deliver.ps1 -Install
#>
[CmdletBinding()]
param(
    [switch]$Release,
    [switch]$Install,
    [ValidatePattern('^$|^[0-9a-f]{40}$')][string]$ExpectedCommit='',
    [string]$PublicationManifest='',
    [string]$InstallDirectory='',
    [string]$StateDirectory=(Join-Path $env:LOCALAPPDATA 'ED Copilot Delivery'),
    [string]$ReportPath='',
    [ValidateRange(1,90)][int]$TimeoutMinutes=45,
    [ValidateRange(10,60)][int]$PollSeconds=60,
    [ValidateRange(0,60)][int]$WaitForExitMinutes=15,
    [switch]$AllowSameVersionRepair
)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'delivery-lib.ps1')
if(-not $Release -and -not $Install){throw 'Choose -Release (verify delivery), -Install, or both.'}
$root=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
if($ExpectedCommit -and $PublicationManifest){throw 'Use either -ExpectedCommit or -PublicationManifest.'}
if($PublicationManifest){
    $publication=Get-Content -LiteralPath (Get-EDSafePath $PublicationManifest) -Raw|ConvertFrom-Json
    $ExpectedCommit=$publication.commit
    if($ExpectedCommit -notmatch '^[0-9a-f]{40}$'){throw 'Publication manifest has no valid published commit.'}
}
if($Release -and -not $ExpectedCommit -and (Test-Path -LiteralPath (Join-Path $root '.git'))){
    $head=& git -C $root rev-parse --verify HEAD
    if($LASTEXITCODE -ne 0 -or "$head".Trim() -notmatch '^[0-9a-f]{40}$'){throw 'Could not resolve the checked-out Git HEAD.'}
    $ExpectedCommit="$head".Trim()
}
if($Release -and -not $ExpectedCommit){throw 'Publish source first, then use its Git checkout, -PublicationManifest, or -ExpectedCommit.'}
$StateDirectory=Get-EDSafePath $StateDirectory
$profile=Join-Path $env:LOCALAPPDATA 'ED Copilot'
if((Test-EDPathOverlap $StateDirectory $profile) -or (Test-EDPathOverlap $StateDirectory $root)){throw 'Delivery state must be separate from source and player profile.'}
if($Install){
    if(-not $InstallDirectory){
        $key='HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\EDCopilot.Desktop_is1'
        $InstallDirectory=if(Test-Path -LiteralPath $key){(Get-ItemProperty -LiteralPath $key).InstallLocation}else{Join-Path $env:LOCALAPPDATA 'Programs/ED Copilot'}
    }
    $InstallDirectory=Get-EDSafePath $InstallDirectory
    if((Test-EDPathOverlap $InstallDirectory $StateDirectory) -or (Test-EDPathOverlap $InstallDirectory $profile) -or (Test-EDPathOverlap $InstallDirectory $root)){throw 'Installation, delivery state, source and player profile must be separate.'}
}
[IO.Directory]::CreateDirectory($StateDirectory)|Out-Null
if(-not $ReportPath){$ReportPath=Join-Path $StateDirectory 'delivery-last.json'}
$ReportPath=Get-EDSafePath $ReportPath
if(-not $ReportPath.StartsWith($StateDirectory.TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetExtension($ReportPath) -ne '.json'){throw 'Delivery report must be a JSON file under the delivery state directory.'}
$result=[ordered]@{ok=$false;status='waiting';expectedCommit=$ExpectedCommit;repository=$script:EDDeliveryRepository;startedAt=[DateTime]::UtcNow.ToString('o')}
try{
    $metadata=$null
    if(-not $ExpectedCommit){
        $metadata=Get-EDPublicJSON 'releases/latest'
        $ExpectedCommit=Resolve-EDReleaseCommit $metadata.tag_name
    }
    $version=if($Release){Get-EDCopilotVersion $root}elseif($null -ne $metadata){$metadata.tag_name -replace '^v',''}else{Get-EDCopilotVersion $root}
    $result.expectedCommit=$ExpectedCommit;$result.version=$version
    $deadline=[DateTime]::UtcNow.AddMinutes($TimeoutMinutes)
    $lastStatus=''
    do{
        if($Release){
            $head=Get-EDPublicJSON 'git/ref/heads/main'
            if($head.object.sha -ne $ExpectedCommit){throw 'A newer source commit superseded this delivery. No older release will be installed.'}
        }
        $runs=Get-EDPublicJSON "actions/workflows/windows.yml/runs?head_sha=$ExpectedCommit&event=push&per_page=10"
        $run=@($runs.workflow_runs|Where-Object {$_.head_sha -eq $ExpectedCommit -and $_.event -eq 'push'}|Sort-Object run_number -Descending|Select-Object -First 1)
        $status=if($run.Count -eq 0){'waiting-for-CI'}elseif($run[0].status -ne 'completed'){'CI-'+$run[0].status}else{'CI-'+$run[0].conclusion}
        if($status -ne $lastStatus){Write-Information "Delivery $version : $status" -InformationAction Continue;$lastStatus=$status}
        if($run.Count -gt 0 -and $run[0].status -eq 'completed'){
            if($run[0].conclusion -ne 'success'){throw "Expected source failed CI: $($run[0].conclusion)."}
            $result.workflowRun=$run[0].html_url
            try{$metadata=Get-EDPublicJSON "releases/tags/v$version"}catch{if($_.Exception.Message -notmatch 'HTTP 404\.$'){throw}}
            if($null -ne $metadata){break}
        }
        if([DateTime]::UtcNow -ge $deadline){throw 'Timed out waiting for the expected verified release.'}
        Start-Sleep -Seconds $PollSeconds
    }while($true)
    if((Resolve-EDReleaseCommit $metadata.tag_name) -ne $ExpectedCommit){throw 'Published release tag points to different source.'}
    $directory=Join-Path $StateDirectory ("releases/$version-"+$ExpectedCommit.Substring(0,12))
    $manifest=Save-EDReleaseAssets -Release $metadata -ExpectedCommit $ExpectedCommit -Directory $directory
    if($Release){$null=Test-EDReleaseDirectory -Directory $directory -ExpectedCommit $ExpectedCommit -SourceRoot $root}
    $result.release=$metadata.html_url;$result.directory=$directory;$result.status='verified';$result.verifiedAssets=1+@($manifest.artifacts | Where-Object published).Count
    if($Install){
        $installed=Install-EDVerifiedRelease -Manifest $manifest -Directory $directory -InstallDirectory $InstallDirectory -StateDirectory $StateDirectory -WaitForExitMinutes $WaitForExitMinutes -AllowSameVersionRepair:$AllowSameVersionRepair
        $result.installation=$installed
        $result.status=if($installed.Contains('deferred')){'deferred'}elseif($installed.alreadyCurrent){'current'}else{'installed'}
    }
    $result.ok=$true
}catch{
    $result.status='failed';$result.error=$_.Exception.Message
    throw
}finally{
    $result.finishedAt=[DateTime]::UtcNow.ToString('o')
    Write-EDDeliveryJSON $ReportPath $result
    Write-Output ($result|ConvertTo-Json -Depth 10)
}
