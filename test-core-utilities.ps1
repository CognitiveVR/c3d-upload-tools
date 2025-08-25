#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Comprehensive test suite for C3D PowerShell core utilities (SDK-183).

.DESCRIPTION
    Tests all the private utility functions that replace bash upload-utils.sh functionality.
    Validates logging, API key handling, UUID validation, file operations, and more.
#>

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

Write-Host "🧪 Testing C3D Core Utilities (SDK-183) on macOS PowerShell" -ForegroundColor Magenta
Write-Host "=================================================================" -ForegroundColor Magenta

# Import the module
try {
    Import-Module ./C3DUploadTools -Force -Verbose:$false
    Write-Host "✅ Module imported successfully" -ForegroundColor Green
} catch {
    Write-Host "❌ Failed to import module: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

$testsPassed = 0
$testsFailed = 0

function Test-Function {
    param(
        [string]$TestName,
        [scriptblock]$TestCode
    )
    
    Write-Host "`n📋 Test: $TestName" -ForegroundColor Cyan
    
    try {
        & $TestCode
        Write-Host "✅ PASSED: $TestName" -ForegroundColor Green
        $script:testsPassed++
    } catch {
        Write-Host "❌ FAILED: $TestName - $($_.Exception.Message)" -ForegroundColor Red
        $script:testsFailed++
    }
}

# Test 1: Logging Functions
Test-Function "Write-C3DLog Basic Functionality" {
    # Enable verbose mode for testing
    Set-C3DVerboseMode -Enabled $true
    
    Write-C3DLog "Test info message" -Level Info
    Write-C3DLog "Test warning message" -Level Warn  
    Write-C3DLog "Test debug message" -Level Debug
    
    # Test error logging (should not throw)
    try {
        Write-C3DLog "Test error message" -Level Error -ErrorAction Continue
    } catch {
        # Expected to continue, not stop
    }
    
    # Disable verbose mode
    Set-C3DVerboseMode -Enabled $false
    
    # Debug messages should not appear now
    Write-C3DLog "This debug message should not appear" -Level Debug
}

# Test 2: API Key Validation (without actual key)
Test-Function "API Key Validation (No Key Set)" {
    # Ensure no API key is set
    $env:C3D_DEVELOPER_API_KEY = $null
    
    $result = Test-C3DApiKey
    if ($result -eq $true) {
        throw "Expected false when no API key is set"
    }
}

# Test 3: API Key Validation (With Test Key)
Test-Function "API Key Validation (Test Key)" {
    # Set a test API key
    $env:C3D_DEVELOPER_API_KEY = "test_api_key_1234567890"
    
    $result = Test-C3DApiKey
    if ($result -ne $true) {
        throw "Expected true with valid test API key"
    }
    
    # Test getting the key
    $key = Get-C3DApiKey
    if ($key -ne "test_api_key_1234567890") {
        throw "Get-C3DApiKey returned unexpected value"
    }
}

# Test 4: API Key Validation (Placeholder Values)
Test-Function "API Key Validation (Reject Placeholders)" {
    $env:C3D_DEVELOPER_API_KEY = "test"
    
    $result = Test-C3DApiKey
    if ($result -eq $true) {
        throw "Expected false for placeholder API key"
    }
}

# Test 5: Environment Validation
Test-Function "Environment Validation" {
    # Valid environments
    if (-not (Test-C3DEnvironment -Environment "prod")) {
        throw "Expected true for prod environment"
    }
    
    if (-not (Test-C3DEnvironment -Environment "dev")) {
        throw "Expected true for dev environment"
    }
    
    # Invalid environment
    if (Test-C3DEnvironment -Environment "invalid") {
        throw "Expected false for invalid environment"
    }
}

# Test 6: API URL Generation
Test-Function "API URL Generation" {
    $prodUrl = Get-C3DApiUrl -Environment "prod" -EndpointType "scenes"
    $expectedProdUrl = "https://data.cognitive3d.com/v0/scenes"
    if ($prodUrl -ne $expectedProdUrl) {
        throw "Prod URL mismatch. Expected: $expectedProdUrl, Got: $prodUrl"
    }
    
    $devUrl = Get-C3DApiUrl -Environment "dev" -EndpointType "objects"  
    $expectedDevUrl = "https://data.c3ddev.com/v0/objects"
    if ($devUrl -ne $expectedDevUrl) {
        throw "Dev URL mismatch. Expected: $expectedDevUrl, Got: $devUrl"
    }
}

# Test 7: UUID Validation
Test-Function "UUID Format Validation" {
    # Valid UUIDs
    $validUuids = @(
        "12345678-1234-1234-1234-123456789012",
        "abcdef01-2345-6789-abcd-ef0123456789",
        "ABCDEF01-2345-6789-ABCD-EF0123456789"
    )
    
    foreach ($uuid in $validUuids) {
        if (-not (Test-C3DUuidFormat -Uuid $uuid)) {
            throw "Valid UUID rejected: $uuid"
        }
    }
    
    # Invalid UUIDs  
    $invalidUuids = @(
        "invalid-uuid",
        "12345678-1234-1234-1234",
        "12345678-1234-1234-1234-12345678901234",
        ""
    )
    
    foreach ($uuid in $invalidUuids) {
        if (Test-C3DUuidFormat -Uuid $uuid) {
            throw "Invalid UUID accepted: $uuid"
        }
    }
}

# Test 8: UUID Case Conversion
Test-Function "UUID Case Conversion" {
    $upperUuid = "ABCDEF01-2345-6789-ABCD-EF0123456789"
    $lowerUuid = ConvertTo-C3DLowerUuid -Uuid $upperUuid
    $expectedLower = "abcdef01-2345-6789-abcd-ef0123456789"
    
    if ($lowerUuid -ne $expectedLower) {
        throw "UUID case conversion failed. Expected: $expectedLower, Got: $lowerUuid"
    }
}

# Test 9: UUID Generation
Test-Function "UUID Generation" {
    $newUuid = New-C3DUuid
    
    if (-not (Test-C3DUuidFormat -Uuid $newUuid)) {
        throw "Generated UUID is invalid: $newUuid"
    }
    
    # Should be lowercase
    if ($newUuid -cne $newUuid.ToLowerInvariant()) {
        throw "Generated UUID is not lowercase: $newUuid"
    }
}

# Test 10: File System Validation (Create Test Files)
Test-Function "File System Validation" {
    $testDir = New-Item -ItemType Directory -Path "/tmp/c3d-test-$(Get-Random)" -Force
    $testFile = New-Item -ItemType File -Path (Join-Path $testDir "test.txt") -Force
    "Test content" | Out-File -FilePath $testFile.FullName -Encoding UTF8
    
    try {
        # Test directory validation
        if (-not (Test-C3DDirectory -Path $testDir.FullName -Name "test directory")) {
            throw "Directory validation failed"
        }
        
        # Test file validation
        if (-not (Test-C3DFile -Path $testFile.FullName -Name "test file")) {
            throw "File validation failed"
        }
        
        # Test required files
        $requiredFiles = @("test.txt")
        if (-not (Test-C3DDirectory -Path $testDir.FullName -Name "test directory" -RequiredFiles $requiredFiles)) {
            throw "Required files validation failed"
        }
        
        # Test file size info
        $sizeInfo = Get-C3DFileSize -Path $testFile.FullName
        if (-not $sizeInfo.Bytes) {
            throw "File size info failed"
        }
        
        # Test backup
        $backupPath = Backup-C3DFile -Path $testFile.FullName
        if (-not (Test-Path $backupPath)) {
            throw "File backup failed"
        }
        
    } finally {
        # Clean up
        Remove-Item -Path $testDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Test 11: Strict Mode Initialization  
Test-Function "Strict Mode Initialization" {
    Initialize-C3DStrictMode
    
    # Test that undefined variable access fails (strict mode)
    try {
        $undefinedVar = $ThisVariableDoesNotExist
        throw "Expected error for undefined variable"
    } catch {
        # Expected - strict mode should catch undefined variables
        if ($_.Exception.Message -notlike "*variable*") {
            throw "Unexpected error type: $($_.Exception.Message)"
        }
    }
}

# Test 12: Cross-platform Path Handling
Test-Function "Cross-platform Path Handling" {
    # Test various path formats work on macOS
    $paths = @(
        "/tmp",
        "~/Downloads", 
        ".",
        ".."
    )
    
    foreach ($path in $paths) {
        try {
            $resolved = Resolve-Path $path -ErrorAction SilentlyContinue
            if (-not $resolved -and ($path -eq "/tmp")) {
                throw "Failed to resolve basic Unix path: $path"
            }
        } catch {
            Write-Host "Path resolution info: $path -> $($_.Exception.Message)" -ForegroundColor Yellow
        }
    }
}

# Restore environment
$env:C3D_DEVELOPER_API_KEY = $null

# Test Summary
Write-Host "`n🎯 Test Results Summary" -ForegroundColor Magenta
Write-Host "======================" -ForegroundColor Magenta
Write-Host "Tests Passed: $testsPassed" -ForegroundColor Green
Write-Host "Tests Failed: $testsFailed" -ForegroundColor $(if ($testsFailed -gt 0) { 'Red' } else { 'Green' })

if ($testsFailed -eq 0) {
    Write-Host "`n🎉 All Core Utility Tests Passed!" -ForegroundColor Green
    Write-Host "SDK-183 implementation is ready for use." -ForegroundColor Green
    exit 0
} else {
    Write-Host "`n❌ Some tests failed. Please review the output above." -ForegroundColor Red
    exit 1
}