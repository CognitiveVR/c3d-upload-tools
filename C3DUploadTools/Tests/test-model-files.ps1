#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Unit tests for Resolve-C3DModelFiles (private): a scene or dynamic object is
    uploaded as exactly one .glb, or as the <base>.gltf + <base>.bin pair.
    Mixing the forms, or two .glb files, is rejected — cvr-se-upload returns a
    400 for the same shapes.
#>

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

# Run from the repo root (parent of C3DUploadTools/Tests)
Set-Location (Join-Path $PSScriptRoot '../..')

Write-Host "🔧 Resolve-C3DModelFiles tests" -ForegroundColor Magenta

# Source every private function (the module loader does the same, recursively)
Get-ChildItem -Path "./C3DUploadTools/Private" -Filter "*.ps1" -Recurse | ForEach-Object { . $_.FullName }

$testsPassed = 0
$testsFailed = 0

function Test-Function {
    param([string]$TestName, [scriptblock]$TestCode)
    try {
        & $TestCode
        Write-Host "✅ PASSED: $TestName" -ForegroundColor Green
        $script:testsPassed++
    } catch {
        Write-Host "❌ FAILED: $TestName - $($_.Exception.Message)" -ForegroundColor Red
        $script:testsFailed++
    }
}

$fixtureRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("c3d-model-files-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixtureRoot | Out-Null

function New-Fixture {
    param([string]$Name, [string[]]$Files)
    $dir = Join-Path $fixtureRoot $Name
    New-Item -ItemType Directory -Path $dir | Out-Null
    foreach ($f in $Files) { Set-Content -Path (Join-Path $dir $f) -Value 'x' -NoNewline }
    return $dir
}

function Assert-Resolves {
    param([string]$Label, [string]$Dir, [string]$BaseName, [switch]$AnyGlb, [string]$Format, [string[]]$FileNames)
    Test-Function $Label {
        $result = Resolve-C3DModelFiles -Directory $Dir -BaseName $BaseName -AnyGlb:$AnyGlb
        if ($result.Format -ne $Format) { throw "expected format '$Format', got '$($result.Format)'" }
        $actual = @($result.Files.Keys) -join ','
        $expected = $FileNames -join ','
        if ($actual -ne $expected) { throw "expected files [$expected], got [$actual]" }
        foreach ($name in $FileNames) {
            $expectedPath = Join-Path $Dir $name
            if ($result.Files[$name] -ne $expectedPath) { throw "expected $name -> $expectedPath, got $($result.Files[$name])" }
        }
    }
}

function Assert-Rejects {
    param([string]$Label, [string]$Dir, [string]$BaseName, [switch]$AnyGlb, [string]$MessageLike)
    Test-Function $Label {
        $threw = $false
        try { Resolve-C3DModelFiles -Directory $Dir -BaseName $BaseName -AnyGlb:$AnyGlb | Out-Null } catch {
            $threw = $true
            if ($MessageLike -and $_.Exception.Message -notlike $MessageLike) { throw "wrong message: $($_.Exception.Message)" }
        }
        if (-not $threw) { throw "expected an error" }
    }
}

try {
    # ---- scenes (-AnyGlb: a single .glb of any name, or scene.gltf + scene.bin)
    $d = New-Fixture 'scene-pair' @('scene.gltf', 'scene.bin', 'screenshot.png')
    Assert-Resolves 'scene: scene.gltf + scene.bin' $d 'scene' -AnyGlb -Format 'gltf' -FileNames @('scene.bin', 'scene.gltf')

    $d = New-Fixture 'scene-glb' @('MyRoom.glb', 'screenshot.png')
    Assert-Resolves 'scene: a single .glb of any name' $d 'scene' -AnyGlb -Format 'glb' -FileNames @('MyRoom.glb')

    $d = New-Fixture 'scene-glb-upper' @('Room.GLB')
    Assert-Resolves 'scene: .GLB extension matched case-insensitively' $d 'scene' -AnyGlb -Format 'glb' -FileNames @('Room.GLB')

    $d = New-Fixture 'scene-two-glb' @('one.glb', 'two.glb')
    Assert-Rejects 'scene: two .glb files are rejected' $d 'scene' -AnyGlb -MessageLike '*2 .glb files*'

    $d = New-Fixture 'scene-mixed' @('scene.glb', 'scene.gltf', 'scene.bin')
    Assert-Rejects 'scene: .glb beside scene.gltf + scene.bin is rejected' $d 'scene' -AnyGlb -MessageLike '*scene.glb*scene.gltf*'

    $d = New-Fixture 'scene-glb-bin' @('scene.glb', 'scene.bin')
    Assert-Rejects 'scene: .glb beside a stray scene.bin is rejected' $d 'scene' -AnyGlb

    $d = New-Fixture 'scene-half' @('scene.gltf')
    Assert-Rejects 'scene: scene.gltf without scene.bin is rejected' $d 'scene' -AnyGlb -MessageLike '*scene.bin*'

    $d = New-Fixture 'scene-none' @('screenshot.png')
    Assert-Rejects 'scene: no model at all is rejected' $d 'scene' -AnyGlb -MessageLike '*.glb*scene.gltf*'

    # ---- objects (named: <base>.glb, or <base>.gltf + <base>.bin)
    $d = New-Fixture 'obj-pair' @('cube.gltf', 'cube.bin', 'cvr_object_thumbnail.png')
    Assert-Resolves 'object: cube.gltf + cube.bin' $d 'cube' -Format 'gltf' -FileNames @('cube.bin', 'cube.gltf')

    $d = New-Fixture 'obj-glb' @('cube.glb', 'cvr_object_thumbnail.png')
    Assert-Resolves 'object: cube.glb' $d 'cube' -Format 'glb' -FileNames @('cube.glb')

    $d = New-Fixture 'obj-mixed' @('cube.glb', 'cube.gltf', 'cube.bin')
    Assert-Rejects 'object: cube.glb beside cube.gltf + cube.bin is rejected' $d 'cube' -MessageLike '*cube.glb*cube.gltf*'

    $d = New-Fixture 'obj-other-glb' @('other.glb')
    Assert-Rejects 'object: a .glb with another base name is not the model' $d 'cube'

    $d = New-Fixture 'obj-none' @('cvr_object_thumbnail.png')
    Assert-Rejects 'object: no model at all is rejected' $d 'cube' -MessageLike '*cube.glb*cube.gltf*'
} finally {
    Remove-Item -Path $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "`nResults: $testsPassed passed, $testsFailed failed" -ForegroundColor $(if ($testsFailed -eq 0) { 'Green' } else { 'Red' })
if ($testsFailed -gt 0) { exit 1 }
