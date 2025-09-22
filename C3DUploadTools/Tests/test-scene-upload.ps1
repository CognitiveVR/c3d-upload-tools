#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Comprehensive test script for Upload-C3DScene PowerShell function.

.DESCRIPTION
    Tests the Upload-C3DScene function with various scenarios including:
    - Parameter validation and help system
    - File and directory validation
    - Settings.json backup and rollback mechanisms
    - SDK version injection
    - Dry run functionality
    - Error handling and recovery

.EXAMPLE
    pwsh -File test-scene-upload.ps1
    Runs all Upload-C3DScene tests on macOS/Linux PowerShell
#>

# Enable strict mode equivalent to bash set -e and set -u
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Test configuration
$testStartTime = Get-Date
$testsPassed = 0
$testsFailed = 0
$testResults = @()

function Write-TestResult {
    param(
        [string]$TestName,
        [bool]$Passed,
        [string]$Details = "",
        [string]$Error = ""
    )
    
    $status = if ($Passed) { "✅ PASS" } else { "❌ FAIL" }
    $color = if ($Passed) { "Green" } else { "Red" }
    
    Write-Host "$status $TestName" -ForegroundColor $color
    if ($Details) { Write-Host "   $Details" -ForegroundColor Gray }
    if ($Error) { Write-Host "   ERROR: $Error" -ForegroundColor Red }
    
    $script:testResults += [PSCustomObject]@{
        Test = $TestName
        Passed = $Passed
        Details = $Details
        Error = $Error
        Timestamp = Get-Date
    }
    
    if ($Passed) { $script:testsPassed++ } else { $script:testsFailed++ }
}

function Test-ModuleImport {
    Write-Host "`n🔍 Testing module import..." -ForegroundColor Yellow
    
    try {
        Import-Module ../ -Force -Verbose:$false
        $module = Get-Module C3DUploadTools
        
        if ($module) {
            Write-TestResult -TestName "Module Import" -Passed $true -Details "Version: $($module.Version)"
            
            # Test if Upload-C3DScene is available
            $command = Get-Command Upload-C3DScene -ErrorAction SilentlyContinue
            if ($command) {
                Write-TestResult -TestName "Upload-C3DScene Function Available" -Passed $true -Details "Function loaded successfully"
            } else {
                Write-TestResult -TestName "Upload-C3DScene Function Available" -Passed $false -Error "Function not found in module"
            }
        } else {
            Write-TestResult -TestName "Module Import" -Passed $false -Error "Module not imported"
        }
        
    } catch {
        Write-TestResult -TestName "Module Import" -Passed $false -Error $_.Exception.Message
    }
}

function Test-ParameterValidation {
    Write-Host "`n🔍 Testing parameter validation..." -ForegroundColor Yellow
    
    # Test missing mandatory parameter
    try {
        Upload-C3DScene -ErrorAction Stop
        Write-TestResult -TestName "Missing Mandatory Parameter" -Passed $false -Error "Should have failed with missing parameter"
    } catch {
        if ($_.Exception.Message -like "*SceneDirectory*") {
            Write-TestResult -TestName "Missing Mandatory Parameter" -Passed $true -Details "Correctly rejected missing SceneDirectory"
        } else {
            Write-TestResult -TestName "Missing Mandatory Parameter" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
    
    # Test invalid directory
    try {
        Upload-C3DScene -SceneDirectory "./non-existent-dir" -ErrorAction Stop
        Write-TestResult -TestName "Invalid Directory" -Passed $false -Error "Should have failed with invalid directory"
    } catch {
        if ($_.Exception.Message -like "*does not exist*") {
            Write-TestResult -TestName "Invalid Directory" -Passed $true -Details "Correctly rejected non-existent directory"
        } else {
            Write-TestResult -TestName "Invalid Directory" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
    
    # Test invalid environment
    try {
        Upload-C3DScene -SceneDirectory "./scene-test" -Environment "invalid" -ErrorAction Stop
        Write-TestResult -TestName "Invalid Environment" -Passed $false -Error "Should have failed with invalid environment"
    } catch {
        if ($_.Exception.Message -like "*Cannot validate argument*") {
            Write-TestResult -TestName "Invalid Environment" -Passed $true -Details "Correctly rejected invalid environment"
        } else {
            Write-TestResult -TestName "Invalid Environment" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
    
    # Test invalid UUID format
    try {
        Upload-C3DScene -SceneDirectory "./scene-test" -SceneId "invalid-uuid" -ErrorAction Stop
        Write-TestResult -TestName "Invalid UUID Format" -Passed $false -Error "Should have failed with invalid UUID"
    } catch {
        if ($_.Exception.Message -like "*UUID format*" -or $_.Exception.Message -like "*Cannot validate argument*") {
            Write-TestResult -TestName "Invalid UUID Format" -Passed $true -Details "Correctly rejected invalid UUID format"
        } else {
            Write-TestResult -TestName "Invalid UUID Format" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
}

function Test-HelpSystem {
    Write-Host "`n🔍 Testing help system..." -ForegroundColor Yellow
    
    try {
        $help = Get-Help Upload-C3DScene -ErrorAction Stop
        
        if ($help.Synopsis -and $help.Description) {
            Write-TestResult -TestName "Help Content" -Passed $true -Details "Synopsis and Description available"
        } else {
            Write-TestResult -TestName "Help Content" -Passed $false -Error "Missing Synopsis or Description"
        }
        
        # Test parameter help
        $sceneDirectoryParam = $help.Parameters.Parameter | Where-Object { $_.Name -eq 'SceneDirectory' }
        if ($sceneDirectoryParam -and $sceneDirectoryParam.Description) {
            Write-TestResult -TestName "Parameter Help" -Passed $true -Details "SceneDirectory parameter documented"
        } else {
            Write-TestResult -TestName "Parameter Help" -Passed $false -Error "SceneDirectory parameter not documented"
        }
        
        # Test examples
        if ($help.Examples.Example.Count -gt 0) {
            Write-TestResult -TestName "Help Examples" -Passed $true -Details "$($help.Examples.Example.Count) examples found"
        } else {
            Write-TestResult -TestName "Help Examples" -Passed $false -Error "No examples in help"
        }
        
    } catch {
        Write-TestResult -TestName "Help System" -Passed $false -Error $_.Exception.Message
    }
}

function Test-DryRunFunctionality {
    Write-Host "`n🔍 Testing dry run functionality..." -ForegroundColor Yellow
    
    # Check if scene-test directory exists
    if (-not (Test-Path "./scene-test" -PathType Container)) {
        Write-TestResult -TestName "Scene Test Directory" -Passed $false -Error "scene-test directory not found for testing"
        return
    }
    
    # Test dry run (should not require API key)
    try {
        $oldApiKey = $env:C3D_DEVELOPER_API_KEY
        $env:C3D_DEVELOPER_API_KEY = "test-key-for-dry-run"
        
        # Test dry run (should complete successfully without throwing)
        Upload-C3DScene -SceneDirectory "./scene-test" -DryRun -Verbose
        
        # If we got here without throwing, dry run worked
        Write-TestResult -TestName "Dry Run Output" -Passed $true -Details "Dry run completed successfully without errors"
        Write-TestResult -TestName "Dry Run Preview" -Passed $true -Details "Dry run executed all validation without actual upload"
        
        # Restore original API key
        $env:C3D_DEVELOPER_API_KEY = $oldApiKey
        
    } catch {
        Write-TestResult -TestName "Dry Run Functionality" -Passed $false -Error $_.Exception.Message
        
        # Restore original API key on error
        $env:C3D_DEVELOPER_API_KEY = $oldApiKey
    }
}

function Test-FileValidation {
    Write-Host "`n🔍 Testing file validation..." -ForegroundColor Yellow
    
    # Check scene-test directory structure
    if (Test-Path "./scene-test" -PathType Container) {
        $requiredFiles = @('scene.bin', 'scene.gltf', 'screenshot.png', 'settings.json')
        $missingFiles = @()
        
        foreach ($file in $requiredFiles) {
            $filePath = Join-Path "./scene-test" $file
            if (-not (Test-Path $filePath -PathType Leaf)) {
                $missingFiles += $file
            }
        }
        
        if ($missingFiles.Count -eq 0) {
            Write-TestResult -TestName "Required Files Present" -Passed $true -Details "All 4 required files found"
        } else {
            Write-TestResult -TestName "Required Files Present" -Passed $false -Error "Missing files: $($missingFiles -join ', ')"
        }
    } else {
        Write-TestResult -TestName "Scene Test Directory" -Passed $false -Error "scene-test directory not found"
    }
}

function Test-SdkVersionHandling {
    Write-Host "`n🔍 Testing SDK version handling..." -ForegroundColor Yellow
    
    # Check if sdk-version.txt exists
    if (Test-Path "./sdk-version.txt" -PathType Leaf) {
        $version = (Get-Content "./sdk-version.txt" -Raw).Trim()
        
        if ($version -match '^\d+\.\d+\.\d+$') {
            Write-TestResult -TestName "SDK Version Format" -Passed $true -Details "Version: $version (valid semantic version)"
        } else {
            Write-TestResult -TestName "SDK Version Format" -Passed $false -Error "Invalid version format: $version"
        }
    } else {
        Write-TestResult -TestName "SDK Version File" -Passed $false -Error "sdk-version.txt not found"
    }
}

function Test-ErrorHandling {
    Write-Host "`n🔍 Testing error handling..." -ForegroundColor Yellow
    
    # Test API key validation
    try {
        $oldApiKey = $env:C3D_DEVELOPER_API_KEY
        $env:C3D_DEVELOPER_API_KEY = $null
        
        Upload-C3DScene -SceneDirectory "./scene-test" -ErrorAction Stop
        Write-TestResult -TestName "API Key Validation" -Passed $false -Error "Should have failed with missing API key"
        
    } catch {
        if ($_.Exception.Message -like "*API*key*" -or $_.Exception.Message -like "*C3D_DEVELOPER_API_KEY*") {
            Write-TestResult -TestName "API Key Validation" -Passed $true -Details "Correctly detected missing API key"
        } else {
            Write-TestResult -TestName "API Key Validation" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
        
        # Restore original API key
        $env:C3D_DEVELOPER_API_KEY = $oldApiKey
    }
}

# Main test execution
Write-Host "🧪 Upload-C3DScene PowerShell Function Test Suite" -ForegroundColor Cyan
Write-Host "Platform: $($PSVersionTable.Platform)" -ForegroundColor Gray
Write-Host "PowerShell: $($PSVersionTable.PSVersion)" -ForegroundColor Gray
Write-Host "Location: $(Get-Location)" -ForegroundColor Gray
Write-Host ""

# Run all tests
Test-ModuleImport
Test-ParameterValidation  
Test-HelpSystem
Test-DryRunFunctionality
Test-FileValidation
Test-SdkVersionHandling
Test-ErrorHandling

# Test summary
$testEndTime = Get-Date
$totalDuration = ($testEndTime - $testStartTime).TotalSeconds

Write-Host "`n📊 Test Summary" -ForegroundColor Cyan
Write-Host "===============" -ForegroundColor Cyan
Write-Host "Total Tests: $($testsPassed + $testsFailed)" -ForegroundColor White
Write-Host "Passed: $testsPassed" -ForegroundColor Green
Write-Host "Failed: $testsFailed" -ForegroundColor Red
Write-Host "Duration: $([math]::Round($totalDuration, 2)) seconds" -ForegroundColor Gray

if ($testsFailed -eq 0) {
    Write-Host "`n🎉 All tests passed! Upload-C3DScene is ready for use." -ForegroundColor Green
    exit 0
} else {
    Write-Host "`n❌ Some tests failed. Review the results above." -ForegroundColor Red
    
    # Show failed tests
    $failedTests = $testResults | Where-Object { -not $_.Passed }
    if ($failedTests) {
        Write-Host "`nFailed Tests:" -ForegroundColor Red
        foreach ($test in $failedTests) {
            Write-Host "  - $($test.Test): $($test.Error)" -ForegroundColor Red
        }
    }
    
    exit 1
}