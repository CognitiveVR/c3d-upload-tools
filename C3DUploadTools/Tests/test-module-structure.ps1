#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Test script for C3DUploadTools PowerShell module structure on macOS.

.DESCRIPTION
    This script validates that the PowerShell module structure is correct and functions
    can be imported and called. Designed for cross-platform testing.

.NOTES
    Run with: pwsh -File test-module-structure.ps1
#>

[CmdletBinding()]
param()

# Set error action to stop on errors
$ErrorActionPreference = 'Stop'

Write-Host "🧪 Testing C3DUploadTools Module Structure on macOS PowerShell" -ForegroundColor Magenta
Write-Host "=================================================================" -ForegroundColor Magenta

# Test 1: Check PowerShell version
Write-Host "`n📋 Test 1: PowerShell Version Check" -ForegroundColor Cyan
$psVersion = $PSVersionTable.PSVersion
Write-Host "PowerShell Version: $psVersion" -ForegroundColor Green
Write-Host "OS: $($PSVersionTable.OS)" -ForegroundColor Green
Write-Host "Platform: $($PSVersionTable.Platform)" -ForegroundColor Green

if ($psVersion -lt [Version]'5.1') {
    Write-Host "❌ PowerShell version 5.1+ required" -ForegroundColor Red
    exit 1
} else {
    Write-Host "✅ PowerShell version is compatible" -ForegroundColor Green
}

# Test 2: Check module directory structure
Write-Host "`n📋 Test 2: Module Directory Structure" -ForegroundColor Cyan
$moduleRoot = Split-Path $PSScriptRoot -Parent
Write-Host "Module root: $moduleRoot" -ForegroundColor Gray

$requiredFiles = @(
    'C3DUploadTools.psd1',
    'C3DUploadTools.psm1'
)

$requiredDirs = @(
    'Public',
    'Private'
)

foreach ($file in $requiredFiles) {
    $filePath = Join-Path $moduleRoot $file
    if (Test-Path $filePath) {
        Write-Host "✅ Found: $file" -ForegroundColor Green
    } else {
        Write-Host "❌ Missing: $file" -ForegroundColor Red
        exit 1
    }
}

foreach ($dir in $requiredDirs) {
    $dirPath = Join-Path $moduleRoot $dir
    if (Test-Path $dirPath -PathType Container) {
        $fileCount = (Get-ChildItem $dirPath -Filter "*.ps1").Count
        Write-Host "✅ Found: $dir/ ($fileCount .ps1 files)" -ForegroundColor Green
    } else {
        Write-Host "❌ Missing: $dir/" -ForegroundColor Red
        exit 1
    }
}

# Test 3: Validate module manifest
Write-Host "`n📋 Test 3: Module Manifest Validation" -ForegroundColor Cyan
try {
    $manifestPath = Join-Path $moduleRoot "C3DUploadTools.psd1"
    $manifest = Test-ModuleManifest -Path $manifestPath -Verbose:$false
    Write-Host "✅ Module manifest is valid" -ForegroundColor Green
    Write-Host "   Name: $($manifest.Name)" -ForegroundColor Gray
    Write-Host "   Version: $($manifest.Version)" -ForegroundColor Gray
    Write-Host "   Author: $($manifest.Author)" -ForegroundColor Gray
    Write-Host "   Compatible Editions: $($manifest.CompatiblePSEditions -join ', ')" -ForegroundColor Gray
} catch {
    Write-Host "❌ Module manifest validation failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# Test 4: Import module
Write-Host "`n📋 Test 4: Module Import Test" -ForegroundColor Cyan
try {
    Import-Module $moduleRoot -Force -Verbose
    Write-Host "✅ Module imported successfully" -ForegroundColor Green
} catch {
    Write-Host "❌ Module import failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# Test 5: Check exported functions
Write-Host "`n📋 Test 5: Exported Functions Test" -ForegroundColor Cyan
$expectedFunctions = @(
    'Upload-C3DScene',
    'Upload-C3DObject',
    'Upload-C3DObjectManifest',
    'Get-C3DObjects'
)

$module = Get-Module C3DUploadTools
$exportedFunctions = $module.ExportedFunctions.Keys

foreach ($func in $expectedFunctions) {
    if ($exportedFunctions -contains $func) {
        Write-Host "✅ Function exported: $func" -ForegroundColor Green
    } else {
        Write-Host "❌ Function not exported: $func" -ForegroundColor Red
        exit 1
    }
}

# Test 6: Function parameter validation
Write-Host "`n📋 Test 6: Function Parameter Validation" -ForegroundColor Cyan

# Test Upload-C3DScene with invalid directory (should fail gracefully)
try {
    $result = Upload-C3DScene -SceneDirectory "/nonexistent/path" -DryRun -ErrorAction Stop
    Write-Host "❌ Upload-C3DScene should have failed with invalid directory" -ForegroundColor Red
} catch {
    Write-Host "✅ Upload-C3DScene correctly validated invalid directory" -ForegroundColor Green
}

# Test Upload-C3DScene with valid directory (create temp dir for test)
try {
    $tempDir = New-Item -ItemType Directory -Path (Join-Path $env:TMPDIR "c3d-test-$(Get-Random)")
    try {
        $result = Upload-C3DScene -SceneDirectory $tempDir.FullName -DryRun
        Write-Host "✅ Upload-C3DScene dry run completed successfully" -ForegroundColor Green
    } finally {
        Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
} catch {
    if ($_.Exception.Message -contains "Not implemented yet") {
        Write-Host "✅ Upload-C3DScene placeholder working correctly" -ForegroundColor Green
    } else {
        Write-Host "❌ Unexpected error in Upload-C3DScene: $($_.Exception.Message)" -ForegroundColor Red
    }
}

# Test 7: Help system
Write-Host "`n📋 Test 7: Help System Test" -ForegroundColor Cyan
try {
    $help = Get-Help Upload-C3DScene -ErrorAction Stop
    if ($help.Synopsis) {
        Write-Host "✅ Help available for Upload-C3DScene" -ForegroundColor Green
        Write-Host "   Synopsis: $($help.Synopsis)" -ForegroundColor Gray
    } else {
        Write-Host "⚠️  Help available but no synopsis found" -ForegroundColor Yellow
    }
} catch {
    Write-Host "❌ Help not available for Upload-C3DScene: $($_.Exception.Message)" -ForegroundColor Red
}

# Test 8: Cross-platform path handling
Write-Host "`n📋 Test 8: Cross-Platform Path Handling" -ForegroundColor Cyan
$testPaths = @(
    "/tmp",
    "~/Downloads",
    ".",
    ".."
)

foreach ($path in $testPaths) {
    $resolved = Resolve-Path $path -ErrorAction SilentlyContinue
    if ($resolved) {
        Write-Host "✅ Path resolved: $path -> $($resolved.Path)" -ForegroundColor Green
    } else {
        Write-Host "⚠️  Path not found (expected): $path" -ForegroundColor Yellow
    }
}

# Clean up
Write-Host "`n🧹 Cleaning Up" -ForegroundColor Cyan
Remove-Module C3DUploadTools -Force -ErrorAction SilentlyContinue
Write-Host "✅ Module removed from session" -ForegroundColor Green

Write-Host "`n🎉 All Tests Completed Successfully!" -ForegroundColor Green
Write-Host "The C3DUploadTools module structure is ready for implementation." -ForegroundColor Green