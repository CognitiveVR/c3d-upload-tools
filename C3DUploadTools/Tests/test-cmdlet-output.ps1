#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Offline tests for what the public cmdlets write to the pipeline: each returns
    exactly one result object, with no stray values from internal validation calls,
    and the scene ID parser reads the gateway's plain-text create response.
    Kept ASCII-only so Windows PowerShell 5.1 parses it without a BOM.
#>

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$originalKey = $env:C3D_DEVELOPER_API_KEY
Push-Location (Join-Path $PSScriptRoot '../..')

$testsPassed = 0
$testsFailed = 0

try {
    Write-Host "Cmdlet output tests" -ForegroundColor Magenta

    # A fake key set before import stops the module loading the real one from .env
    $env:C3D_DEVELOPER_API_KEY = 'offline-test-key-not-a-real-key'
    Import-Module ./C3DUploadTools -Force -WarningAction SilentlyContinue
    . ./C3DUploadTools/Private/Api/Get-C3DSceneIdFromResponse.ps1

    function Test-Function {
        param([string]$TestName, [scriptblock]$TestCode)
        try {
            & $TestCode
            Write-Host "PASSED: $TestName" -ForegroundColor Green
            $script:testsPassed++
        } catch {
            Write-Host "FAILED: $TestName - $($_.Exception.Message)" -ForegroundColor Red
            $script:testsFailed++
        }
    }

    function Assert-SingleResult {
        param($Output, [string]$Cmdlet)
        $items = @($Output)
        if ($items.Count -ne 1) {
            throw "$Cmdlet wrote $($items.Count) items to the pipeline: $($items | ConvertTo-Json -Compress -Depth 2)"
        }
        if ($items[0] -is [bool]) {
            throw "$Cmdlet returned a bare boolean instead of a result object"
        }
        return $items[0]
    }

    $sceneId = '70C4F65A-FBE8-4AD5-8D1C-7670798969E9'
    $uuidPattern = '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'

    Test-Function "Upload-C3DScene -DryRun (new .glb scene) returns one result object" {
        $out = Upload-C3DScene -SceneDirectory ./scene-test-glb -SceneName 'offline-test' -Environment dev -DryRun 6>$null
        $r = Assert-SingleResult $out 'Upload-C3DScene'
        if (-not $r.Success) { throw "Success should be true" }
        if (-not $r.DryRun) { throw "DryRun should be true" }
        if (-not $r.Created) { throw "Created should be true when no SceneId is given" }
        if ($null -ne $r.SceneId) { throw "SceneId should be null on a dry run of a new scene, got '$($r.SceneId)'" }
        if ($r.Environment -ne 'dev') { throw "Environment should be 'dev', got '$($r.Environment)'" }
    }

    Test-Function "Upload-C3DScene -DryRun -SceneId returns the lowercased scene ID" {
        $out = Upload-C3DScene -SceneDirectory ./scene-test -SceneId $sceneId -Environment dev -DryRun 6>$null
        $r = Assert-SingleResult $out 'Upload-C3DScene'
        if ($r.SceneId -ne $sceneId.ToLower()) { throw "SceneId should be '$($sceneId.ToLower())', got '$($r.SceneId)'" }
        if ($r.Created) { throw "Created should be false when updating a scene" }
    }

    Test-Function "Upload-C3DObject -DryRun (.glb) returns one result object with an ObjectId" {
        $out = Upload-C3DObject -SceneId $sceneId -ObjectFilename lantern -ObjectDirectory ./object-test-glb -Environment dev -DryRun 6>$null
        $r = Assert-SingleResult $out 'Upload-C3DObject'
        if (-not $r.Success) { throw "Success should be true" }
        if ($r.ObjectId -notmatch $uuidPattern) { throw "ObjectId should be a lowercase UUID, got '$($r.ObjectId)'" }
        if ($r.SceneId -ne $sceneId.ToLower()) { throw "SceneId should be lowercased, got '$($r.SceneId)'" }
        if ($r.ObjectFilename -ne 'lantern') { throw "ObjectFilename should be 'lantern', got '$($r.ObjectFilename)'" }
    }

    Test-Function "Upload-C3DObject -DryRun (.gltf + .bin) returns one result object" {
        $out = Upload-C3DObject -SceneId $sceneId -ObjectFilename cube -ObjectDirectory ./object-test -Environment dev -DryRun 6>$null
        $null = Assert-SingleResult $out 'Upload-C3DObject'
    }

    $manifestDir = Join-Path ([System.IO.Path]::GetTempPath()) ("c3d-cmdlet-output-" + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $manifestDir | Out-Null
    Set-Content -Path (Join-Path $manifestDir "$($sceneId.ToLower())_object_manifest.json") -Value '{"objects":[]}'

    try {
        Push-Location $manifestDir

        Test-Function "Upload-C3DObjectManifest -DryRun returns one result object" {
            $out = Upload-C3DObjectManifest -SceneId $sceneId.ToLower() -Environment dev -DryRun 6>$null
            $r = Assert-SingleResult $out 'Upload-C3DObjectManifest'
            if (-not $r.DryRun) { throw "DryRun should be true" }
        }

        Test-Function "Upload-C3DObjectManifest stops when the API key is missing" {
            $saved = $env:C3D_DEVELOPER_API_KEY
            $env:C3D_DEVELOPER_API_KEY = ''
            try {
                $threw = $false
                try { Upload-C3DObjectManifest -SceneId $sceneId.ToLower() -Environment dev -DryRun 6>$null *>$null } catch { $threw = $true }
                if (-not $threw) { throw "expected an error for a missing API key" }
            } finally {
                $env:C3D_DEVELOPER_API_KEY = $saved
            }
        }
    } finally {
        Pop-Location
        Remove-Item -Path $manifestDir -Recurse -Force
    }

    Test-Function "Get-C3DObjects stops when the API key is missing" {
        $saved = $env:C3D_DEVELOPER_API_KEY
        $env:C3D_DEVELOPER_API_KEY = ''
        try {
            $threw = $false
            $message = ''
            try { Get-C3DObjects -SceneId $sceneId -Environment dev 6>$null *>$null } catch { $threw = $true; $message = $_.Exception.Message }
            if (-not $threw) { throw "expected an error for a missing API key" }
            if ($message -notmatch 'C3D_DEVELOPER_API_KEY') { throw "expected the missing-key error, got: $message" }
        } finally {
            $env:C3D_DEVELOPER_API_KEY = $saved
        }
    }

    Test-Function "Get-C3DSceneIdFromResponse reads a quoted, bare or BOM-prefixed scene ID" {
        $expected = '70c4f65a-fbe8-4ad5-8d1c-7670798969e9'
        $bom = [string][char]0xFEFF
        foreach ($body in @("`"$expected`"", $expected, "  $expected`r`n", "`"$($expected.ToUpper())`"", "$bom$expected", "$bom`"$expected`"")) {
            $got = Get-C3DSceneIdFromResponse -Body $body
            if ($got -ne $expected) { throw "body '$body' gave '$got'" }
        }
    }

    Test-Function "Get-C3DSceneIdFromResponse returns null for an empty or non-UUID body" {
        foreach ($body in @($null, '', '   ', '{"id":"x"}', 'not-a-uuid')) {
            $got = Get-C3DSceneIdFromResponse -Body $body
            if ($null -ne $got) { throw "body '$body' gave '$got', expected null" }
        }
    }

    # The tests below replace Invoke-C3DApiRequest inside the module, so nothing reaches the network
    $module = Get-Module C3DUploadTools
    $workDir = Join-Path ([System.IO.Path]::GetTempPath()) ("c3d-cmdlet-mock-" + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $workDir | Out-Null
    $objectDir = (Resolve-Path ./object-test-glb).Path
    $sceneDir = (Resolve-Path ./scene-test-glb).Path

    try {
        Push-Location $workDir

        Test-Function "Upload-C3DObject reports a failed manifest upload as not uploaded" {
            & $module {
                Set-Item function:script:Invoke-C3DApiRequest {
                    param($Uri, $Method, $FormData, $Body, $ContentType, $ApiKey, $ProgressTitle)
                    if ($FormData) {
                        [PSCustomObject]@{ Success = $true; StatusCode = 200; Body = ''; Error = $null }
                    } else {
                        [PSCustomObject]@{ Success = $false; StatusCode = 500; Body = 'mock failure'; Error = 'mock failure' }
                    }
                }
            }
            $all = Upload-C3DObject -SceneId $sceneId -ObjectFilename lantern -ObjectDirectory $objectDir -Environment dev 6>&1
            $hostText = ($all | Where-Object { $_ -is [System.Management.Automation.InformationRecord] }) -join "`n"
            $r = Assert-SingleResult @($all | Where-Object { $_ -isnot [System.Management.Automation.InformationRecord] }) 'Upload-C3DObject'
            if ($r.ManifestUploaded) { throw "ManifestUploaded should be false when the manifest upload fails" }
            if ($hostText -match 'automatically uploaded') { throw "console claims the manifest was uploaded" }
        }

        Test-Function "Upload-C3DScene warns when a create response has no scene ID" {
            & $module {
                Set-Item function:script:Invoke-C3DApiRequest {
                    param($Uri, $Method, $FormData, $Body, $ContentType, $ApiKey, $ProgressTitle)
                    [PSCustomObject]@{ Success = $true; StatusCode = 200; Body = ''; Error = $null }
                }
            }
            $sceneCopy = Join-Path $workDir 'scene'
            Copy-Item -Path $sceneDir -Destination $sceneCopy -Recurse
            $warnings = @()
            $out = Upload-C3DScene -SceneDirectory $sceneCopy -SceneName 'offline-test' -Environment dev -WarningVariable warnings -WarningAction SilentlyContinue 6>$null
            $r = Assert-SingleResult $out 'Upload-C3DScene'
            if ($null -ne $r.SceneId) { throw "SceneId should be null, got '$($r.SceneId)'" }
            if (@($warnings).Count -lt 1 -or "$warnings" -notmatch 'no scene ID') { throw "expected a PowerShell warning about the missing scene ID" }
        }
    } finally {
        Pop-Location
        # Re-import before cleanup so a failed Remove-Item cannot leave the mock installed
        Import-Module ./C3DUploadTools -Force -WarningAction SilentlyContinue
        Remove-Item -Path $workDir -Recurse -Force -ErrorAction Continue
    }
} finally {
    $env:C3D_DEVELOPER_API_KEY = $originalKey
    Pop-Location
}

Write-Host ""
Write-Host "Passed: $testsPassed  Failed: $testsFailed"
if ($testsFailed -gt 0) { exit 1 }
exit 0
