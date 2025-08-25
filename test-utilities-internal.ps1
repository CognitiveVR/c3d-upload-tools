#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Internal test suite for C3D PowerShell core utilities (direct function testing).

.DESCRIPTION
    Tests core utilities by directly sourcing the private functions.
    This allows testing internal functionality without module export concerns.
#>

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

Write-Host "🔧 Internal Testing of C3D Core Utilities" -ForegroundColor Magenta
Write-Host "============================================" -ForegroundColor Magenta

# Source all private functions directly
$privateFiles = Get-ChildItem -Path "./C3DUploadTools/Private/*.ps1"
foreach ($file in $privateFiles) {
    Write-Host "Sourcing: $($file.Name)" -ForegroundColor Gray
    . $file.FullName
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
        Write-Host "   Stack: $($_.ScriptStackTrace)" -ForegroundColor Yellow
        $script:testsFailed++
    }
}

# Initialize module state
Initialize-C3DStrictMode
Set-C3DVerboseMode -Enabled $true

# Test 1: Logging Functions
Test-Function "Write-C3DLog Basic Functionality" {
    Write-C3DLog "Test info message" -Level Info
    Write-C3DLog "Test warning message" -Level Warn  
    Write-C3DLog "Test debug message" -Level Debug
    
    # Test that debug messages work with verbose mode enabled
    $script:VerboseMode = $true
    Write-C3DLog "This debug message should appear" -Level Debug
    
    $script:VerboseMode = $false
    Write-C3DLog "This debug message should NOT appear" -Level Debug
}

# Test 2: API Key Validation
Test-Function "API Key Validation" {
    # Clear API key
    $env:C3D_DEVELOPER_API_KEY = $null
    
    $result = Test-C3DApiKey
    if ($result -ne $false) {
        throw "Expected false when no API key is set"
    }
    
    # Set valid test key
    $env:C3D_DEVELOPER_API_KEY = "valid_test_key_1234567890abcdef"
    
    $result = Test-C3DApiKey
    if ($result -ne $true) {
        throw "Expected true with valid API key"
    }
    
    # Test placeholder rejection
    $env:C3D_DEVELOPER_API_KEY = "test"
    $result = Test-C3DApiKey
    if ($result -ne $false) {
        throw "Expected false for placeholder API key"
    }
}

# Test 3: Environment Validation
Test-Function "Environment Validation" {
    if (-not (Test-C3DEnvironment -Environment "prod")) {
        throw "Expected true for prod environment"
    }
    
    if (-not (Test-C3DEnvironment -Environment "dev")) {
        throw "Expected true for dev environment"
    }
    
    if (Test-C3DEnvironment -Environment "invalid") {
        throw "Expected false for invalid environment"
    }
}

# Test 4: API URL Generation
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

# Test 5: UUID Validation
Test-Function "UUID Format Validation" {
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

# Test 6: UUID Operations
Test-Function "UUID Operations" {
    # Test case conversion
    $upperUuid = "ABCDEF01-2345-6789-ABCD-EF0123456789"
    $lowerUuid = ConvertTo-C3DLowerUuid -Uuid $upperUuid
    $expectedLower = "abcdef01-2345-6789-abcd-ef0123456789"
    
    if ($lowerUuid -ne $expectedLower) {
        throw "UUID case conversion failed. Expected: $expectedLower, Got: $lowerUuid"
    }
    
    # Test UUID generation
    $newUuid = New-C3DUuid
    
    if (-not (Test-C3DUuidFormat -Uuid $newUuid)) {
        throw "Generated UUID is invalid: $newUuid"
    }
    
    if ($newUuid -cne $newUuid.ToLowerInvariant()) {
        throw "Generated UUID is not lowercase: $newUuid"
    }
}

# Test 7: File System Operations
Test-Function "File System Operations" {
    $testDir = New-Item -ItemType Directory -Path "/tmp/c3d-internal-test-$(Get-Random)" -Force
    $testFile = New-Item -ItemType File -Path (Join-Path $testDir "test.txt") -Force
    "Test content for internal testing" | Out-File -FilePath $testFile.FullName -Encoding UTF8
    
    try {
        # Test directory validation
        if (-not (Test-C3DDirectory -Path $testDir.FullName -Name "test directory")) {
            throw "Directory validation failed"
        }
        
        # Test nonexistent directory
        if (Test-C3DDirectory -Path "/nonexistent/path" -Name "fake directory") {
            throw "Expected false for nonexistent directory"
        }
        
        # Test file validation
        if (-not (Test-C3DFile -Path $testFile.FullName -Name "test file")) {
            throw "File validation failed"
        }
        
        # Test required files validation
        $requiredFiles = @("test.txt")
        if (-not (Test-C3DDirectory -Path $testDir.FullName -Name "test directory" -RequiredFiles $requiredFiles)) {
            throw "Required files validation failed"
        }
        
        # Test missing required files
        $missingFiles = @("test.txt", "missing.txt")
        if (Test-C3DDirectory -Path $testDir.FullName -Name "test directory" -RequiredFiles $missingFiles) {
            throw "Expected false when required files are missing"
        }
        
        # Test file size info
        $sizeInfo = Get-C3DFileSize -Path $testFile.FullName
        if (-not $sizeInfo.Bytes -or $sizeInfo.Bytes -le 0) {
            throw "File size info failed: $($sizeInfo | ConvertTo-Json)"
        }
        
        # Test file backup
        $backupPath = Backup-C3DFile -Path $testFile.FullName
        if (-not (Test-Path $backupPath)) {
            throw "File backup failed - backup not created at: $backupPath"
        }
        
        if (-not $backupPath.EndsWith('.bak')) {
            throw "Backup path should end with .bak: $backupPath"
        }
        
    } finally {
        # Clean up
        Remove-Item -Path $testDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Test Summary
Write-Host "`n🎯 Internal Test Results" -ForegroundColor Magenta
Write-Host "========================" -ForegroundColor Magenta
Write-Host "Tests Passed: $testsPassed" -ForegroundColor Green
Write-Host "Tests Failed: $testsFailed" -ForegroundColor $(if ($testsFailed -gt 0) { 'Red' } else { 'Green' })

if ($testsFailed -eq 0) {
    Write-Host "`n🎉 All Internal Utility Tests Passed!" -ForegroundColor Green
    Write-Host "Core utilities are working correctly." -ForegroundColor Green
} else {
    Write-Host "`n❌ Some internal tests failed." -ForegroundColor Red
    exit 1
}

# Clean up environment
$env:C3D_DEVELOPER_API_KEY = $null