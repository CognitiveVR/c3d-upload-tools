#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Comprehensive test script for Upload-C3DObject PowerShell function.

.DESCRIPTION
    Tests the Upload-C3DObject function with various scenarios including:
    - Parameter validation and help system
    - Object file and directory validation
    - Texture file discovery and processing
    - Object manifest generation
    - Dry run functionality
    - Error handling and recovery

.EXAMPLE
    pwsh -File test-object-upload.ps1
    Runs all Upload-C3DObject tests on macOS/Linux PowerShell
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
        Import-Module ./C3DUploadTools -Force -Verbose:$false
        $module = Get-Module C3DUploadTools
        
        if ($module) {
            Write-TestResult -TestName "Module Import" -Passed $true -Details "Version: $($module.Version)"
            
            # Test if Upload-C3DObject is available
            $command = Get-Command Upload-C3DObject -ErrorAction SilentlyContinue
            if ($command) {
                Write-TestResult -TestName "Upload-C3DObject Function Available" -Passed $true -Details "Function loaded successfully"
            } else {
                Write-TestResult -TestName "Upload-C3DObject Function Available" -Passed $false -Error "Function not found in module"
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
    
    # Test missing mandatory parameters
    try {
        Upload-C3DObject -ErrorAction Stop
        Write-TestResult -TestName "Missing Mandatory Parameters" -Passed $false -Error "Should have failed with missing parameters"
    } catch {
        if ($_.Exception.Message -like "*SceneId*") {
            Write-TestResult -TestName "Missing Mandatory Parameters" -Passed $true -Details "Correctly rejected missing SceneId"
        } else {
            Write-TestResult -TestName "Missing Mandatory Parameters" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
    
    # Test invalid scene ID UUID
    try {
        Upload-C3DObject -SceneId "invalid-uuid" -ObjectFilename "test" -ObjectDirectory "./object-test" -ErrorAction Stop
        Write-TestResult -TestName "Invalid Scene ID UUID" -Passed $false -Error "Should have failed with invalid scene UUID"
    } catch {
        if ($_.Exception.Message -like "*UUID format*" -or $_.Exception.Message -like "*Cannot validate argument*") {
            Write-TestResult -TestName "Invalid Scene ID UUID" -Passed $true -Details "Correctly rejected invalid scene UUID"
        } else {
            Write-TestResult -TestName "Invalid Scene ID UUID" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
    
    # Test invalid object directory
    try {
        Upload-C3DObject -SceneId "12345678-1234-1234-1234-123456789012" -ObjectFilename "test" -ObjectDirectory "./non-existent-dir" -ErrorAction Stop
        Write-TestResult -TestName "Invalid Object Directory" -Passed $false -Error "Should have failed with invalid directory"
    } catch {
        if ($_.Exception.Message -like "*does not exist*") {
            Write-TestResult -TestName "Invalid Object Directory" -Passed $true -Details "Correctly rejected non-existent directory"
        } else {
            Write-TestResult -TestName "Invalid Object Directory" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
    
    # Test invalid object ID UUID (if provided)
    try {
        $oldApiKey = $env:C3D_DEVELOPER_API_KEY
        $env:C3D_DEVELOPER_API_KEY = "test-key-for-validation"
        
        Upload-C3DObject -SceneId "12345678-1234-1234-1234-123456789012" -ObjectFilename "test" -ObjectDirectory "./object-test" -ObjectId "invalid-object-uuid" -ErrorAction Stop
        Write-TestResult -TestName "Invalid Object ID UUID" -Passed $false -Error "Should have failed with invalid object UUID"
        
        $env:C3D_DEVELOPER_API_KEY = $oldApiKey
    } catch {
        $env:C3D_DEVELOPER_API_KEY = $oldApiKey
        if ($_.Exception.Message -like "*UUID format*" -or $_.Exception.Message -like "*Invalid UUID format for ObjectId*") {
            Write-TestResult -TestName "Invalid Object ID UUID" -Passed $true -Details "Correctly rejected invalid object UUID"
        } else {
            Write-TestResult -TestName "Invalid Object ID UUID" -Passed $false -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
}

function Test-HelpSystem {
    Write-Host "`n🔍 Testing help system..." -ForegroundColor Yellow
    
    try {
        $help = Get-Help Upload-C3DObject -ErrorAction Stop
        
        if ($help.Synopsis -and $help.Description) {
            Write-TestResult -TestName "Help Content" -Passed $true -Details "Synopsis and Description available"
        } else {
            Write-TestResult -TestName "Help Content" -Passed $false -Error "Missing Synopsis or Description"
        }
        
        # Test parameter help
        $sceneIdParam = $help.Parameters.Parameter | Where-Object { $_.Name -eq 'SceneId' }
        if ($sceneIdParam -and $sceneIdParam.Description) {
            Write-TestResult -TestName "Parameter Help" -Passed $true -Details "SceneId parameter documented"
        } else {
            Write-TestResult -TestName "Parameter Help" -Passed $false -Error "SceneId parameter not documented"
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

function Test-ObjectDirectoryValidation {
    Write-Host "`n🔍 Testing object directory validation..." -ForegroundColor Yellow
    
    # Check if object-test directory exists
    if (Test-Path "./object-test" -PathType Container) {
        $requiredFiles = @('cvr_object_thumbnail.png')
        $foundFiles = @()
        $missingFiles = @()
        
        # Check for thumbnail (always required)
        $thumbnailPath = Join-Path "./object-test" "cvr_object_thumbnail.png"
        if (Test-Path $thumbnailPath -PathType Leaf) {
            $foundFiles += "cvr_object_thumbnail.png"
        } else {
            $missingFiles += "cvr_object_thumbnail.png"
        }
        
        # Look for any .gltf and .bin files as examples
        $gltfFiles = Get-ChildItem "./object-test" -Filter "*.gltf" -File
        $binFiles = Get-ChildItem "./object-test" -Filter "*.bin" -File
        
        $gltfCount = @($gltfFiles).Count
        $binCount = @($binFiles).Count
        
        if ($gltfCount -gt 0) {
            Write-TestResult -TestName "GLTF Files Present" -Passed $true -Details "$gltfCount .gltf file(s) found"
        } else {
            Write-TestResult -TestName "GLTF Files Present" -Passed $false -Error "No .gltf files found in object-test"
        }
        
        if ($binCount -gt 0) {
            Write-TestResult -TestName "Binary Files Present" -Passed $true -Details "$binCount .bin file(s) found"
        } else {
            Write-TestResult -TestName "Binary Files Present" -Passed $false -Error "No .bin files found in object-test"
        }
        
        if ($missingFiles.Count -eq 0) {
            Write-TestResult -TestName "Required Object Files" -Passed $true -Details "Thumbnail file found"
        } else {
            Write-TestResult -TestName "Required Object Files" -Passed $false -Error "Missing files: $($missingFiles -join ', ')"
        }
        
    } else {
        Write-TestResult -TestName "Object Test Directory" -Passed $false -Error "object-test directory not found"
    }
}

function Test-DryRunFunctionality {
    Write-Host "`n🔍 Testing dry run functionality..." -ForegroundColor Yellow
    
    # Check if we can find a suitable object for testing
    if (-not (Test-Path "./object-test" -PathType Container)) {
        Write-TestResult -TestName "Object Test Directory Available" -Passed $false -Error "object-test directory not found for testing"
        return
    }
    
    # Find first .gltf file to use as test object
    $gltfFiles = Get-ChildItem "./object-test" -Filter "*.gltf" -File
    $gltfCount = @($gltfFiles).Count
    if ($gltfCount -eq 0) {
        Write-TestResult -TestName "Test Object Available" -Passed $false -Error "No .gltf files found for testing"
        return
    }
    
    $testObjectFilename = [System.IO.Path]::GetFileNameWithoutExtension($gltfFiles[0].Name)
    
    # Test dry run (should not require valid API key for dry run)
    try {
        $oldApiKey = $env:C3D_DEVELOPER_API_KEY
        $env:C3D_DEVELOPER_API_KEY = "test-key-for-dry-run-validation"
        
        # Test dry run
        Upload-C3DObject -SceneId "12345678-1234-1234-1234-123456789012" -ObjectFilename $testObjectFilename -ObjectDirectory "./object-test" -DryRun -Verbose
        
        # If we got here without throwing, dry run worked
        Write-TestResult -TestName "Dry Run Execution" -Passed $true -Details "Dry run completed successfully without errors"
        
        # Restore original API key
        $env:C3D_DEVELOPER_API_KEY = $oldApiKey
        
    } catch {
        Write-TestResult -TestName "Dry Run Execution" -Passed $false -Error $_.Exception.Message
        
        # Restore original API key on error
        $env:C3D_DEVELOPER_API_KEY = $oldApiKey
    }
}

function Test-ErrorHandling {
    Write-Host "`n🔍 Testing error handling..." -ForegroundColor Yellow
    
    # Test API key validation
    try {
        $oldApiKey = $env:C3D_DEVELOPER_API_KEY
        $env:C3D_DEVELOPER_API_KEY = $null
        
        Upload-C3DObject -SceneId "12345678-1234-1234-1234-123456789012" -ObjectFilename "test" -ObjectDirectory "./object-test" -ErrorAction Stop
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

function Test-ObjectIdHandling {
    Write-Host "`n🔍 Testing object ID handling..." -ForegroundColor Yellow
    
    # This is a logical test - we'll test that the function accepts the parameters correctly
    try {
        # Test with ObjectId provided (should validate UUID format)
        $validUuid = "87654321-4321-4321-4321-210987654321"
        
        # We'll use a dummy test that validates parameters without going to API
        $oldApiKey = $env:C3D_DEVELOPER_API_KEY
        $env:C3D_DEVELOPER_API_KEY = "test-key-for-validation"
        
        # Find actual object filename from object-test directory
        $gltfFiles = Get-ChildItem "./object-test" -Filter "*.gltf" -File
        if (@($gltfFiles).Count -eq 0) {
            Write-TestResult -TestName "Valid Object ID Acceptance" -Passed $false -Error "No test objects available"
            return
        }
        $actualObjectFilename = [System.IO.Path]::GetFileNameWithoutExtension($gltfFiles[0].Name)
        
        # This should work if parameter validation is correct
        try {
            Upload-C3DObject -SceneId "12345678-1234-1234-1234-123456789012" -ObjectFilename $actualObjectFilename -ObjectDirectory "./object-test" -ObjectId $validUuid -DryRun -ErrorAction Stop
            Write-TestResult -TestName "Valid Object ID Acceptance" -Passed $true -Details "Valid UUID accepted for ObjectId"
        } catch {
            if ($_.Exception.Message -like "*file*" -or $_.Exception.Message -like "*directory*") {
                # Expected if files don't exist - parameter validation worked
                Write-TestResult -TestName "Valid Object ID Acceptance" -Passed $true -Details "Valid UUID accepted (file validation triggered)"
            } else {
                Write-TestResult -TestName "Valid Object ID Acceptance" -Passed $false -Error $_.Exception.Message
            }
        }
        
        # Restore API key
        $env:C3D_DEVELOPER_API_KEY = $oldApiKey
        
    } catch {
        Write-TestResult -TestName "Object ID Handling" -Passed $false -Error $_.Exception.Message
    }
}

# Main test execution
Write-Host "🧪 Upload-C3DObject PowerShell Function Test Suite" -ForegroundColor Cyan
Write-Host "Platform: $($PSVersionTable.Platform)" -ForegroundColor Gray
Write-Host "PowerShell: $($PSVersionTable.PSVersion)" -ForegroundColor Gray
Write-Host "Location: $(Get-Location)" -ForegroundColor Gray
Write-Host ""

# Run all tests
Test-ModuleImport
Test-ParameterValidation
Test-HelpSystem
Test-ObjectDirectoryValidation
Test-DryRunFunctionality
Test-ErrorHandling
Test-ObjectIdHandling

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
    Write-Host "`n🎉 All tests passed! Upload-C3DObject is ready for use." -ForegroundColor Green
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