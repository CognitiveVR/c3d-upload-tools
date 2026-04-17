#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Tests that Set-C3DRequestHeaders applies headers correctly on both WebClient and HttpWebRequest.

.DESCRIPTION
    Instantiates real .NET objects (no mocks) to verify that restricted headers like User-Agent
    are set via the correct API on each request type. This catches the Windows-specific crash
    where Headers.Add('User-Agent', ...) throws on restricted headers.

.NOTES
    Run from the repo root: pwsh -File C3DUploadTools/Tests/test-http-headers.ps1
#>

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

Write-Host "HTTP Header Assignment Tests" -ForegroundColor Magenta
Write-Host "============================" -ForegroundColor Magenta

# Source only what this test needs
. "$PSScriptRoot/../Private/Core/Write-C3DLog.ps1"
. "$PSScriptRoot/../Private/Api/Set-C3DRequestHeaders.ps1"

$testsPassed = 0
$testsFailed = 0

function Test-Case {
    param([string]$Name, [scriptblock]$Code)
    Write-Host "`nTest: $Name" -ForegroundColor Cyan
    try {
        & $Code
        Write-Host "PASSED" -ForegroundColor Green
        $script:testsPassed++
    } catch {
        Write-Host "FAILED: $($_.Exception.Message)" -ForegroundColor Red
        $script:testsFailed++
    }
}

# --- WebClient tests ---

Test-Case "WebClient: User-Agent set without throwing" {
    $wc = New-Object System.Net.WebClient
    try {
        Set-C3DRequestHeaders -Request $wc -Headers @{ 'User-Agent' = 'C3DUploadTools-PowerShell/1.0' }
        $actual = $wc.Headers[[System.Net.HttpRequestHeader]::UserAgent]
        if ($actual -ne 'C3DUploadTools-PowerShell/1.0') {
            throw "Expected 'C3DUploadTools-PowerShell/1.0', got '$actual'"
        }
    } finally { $wc.Dispose() }
}

Test-Case "WebClient: non-restricted headers added normally" {
    $wc = New-Object System.Net.WebClient
    try {
        Set-C3DRequestHeaders -Request $wc -Headers @{ 'X-Custom-Header' = 'test-value' }
        $actual = $wc.Headers['X-Custom-Header']
        if ($actual -ne 'test-value') {
            throw "Expected 'test-value', got '$actual'"
        }
    } finally { $wc.Dispose() }
}

Test-Case "WebClient: mixed headers (User-Agent + custom) both set correctly" {
    $wc = New-Object System.Net.WebClient
    try {
        Set-C3DRequestHeaders -Request $wc -Headers @{
            'User-Agent'     = 'C3DUploadTools-PowerShell/1.0'
            'X-Custom-Header' = 'custom-value'
        }
        $ua = $wc.Headers[[System.Net.HttpRequestHeader]::UserAgent]
        $custom = $wc.Headers['X-Custom-Header']
        if ($ua -ne 'C3DUploadTools-PowerShell/1.0') { throw "User-Agent mismatch: '$ua'" }
        if ($custom -ne 'custom-value') { throw "X-Custom-Header mismatch: '$custom'" }
    } finally { $wc.Dispose() }
}

# --- HttpWebRequest tests ---

Test-Case "HttpWebRequest: User-Agent set without throwing" {
    $req = [System.Net.HttpWebRequest][System.Net.WebRequest]::Create('https://data.cognitive3d.com')
    Set-C3DRequestHeaders -Request $req -Headers @{ 'User-Agent' = 'C3DUploadTools-PowerShell/1.0' }
    if ($req.UserAgent -ne 'C3DUploadTools-PowerShell/1.0') {
        throw "Expected 'C3DUploadTools-PowerShell/1.0', got '$($req.UserAgent)'"
    }
}

Test-Case "HttpWebRequest: non-restricted headers added normally" {
    $req = [System.Net.HttpWebRequest][System.Net.WebRequest]::Create('https://data.cognitive3d.com')
    Set-C3DRequestHeaders -Request $req -Headers @{ 'X-Custom-Header' = 'test-value' }
    $actual = $req.Headers['X-Custom-Header']
    if ($actual -ne 'test-value') {
        throw "Expected 'test-value', got '$actual'"
    }
}

Test-Case "HttpWebRequest: mixed headers both set correctly" {
    $req = [System.Net.HttpWebRequest][System.Net.WebRequest]::Create('https://data.cognitive3d.com')
    Set-C3DRequestHeaders -Request $req -Headers @{
        'User-Agent'      = 'C3DUploadTools-PowerShell/1.0'
        'X-Custom-Header' = 'custom-value'
    }
    if ($req.UserAgent -ne 'C3DUploadTools-PowerShell/1.0') { throw "User-Agent mismatch: '$($req.UserAgent)'" }
    if ($req.Headers['X-Custom-Header'] -ne 'custom-value') { throw "X-Custom-Header mismatch" }
}

# --- Unknown type test ---

Test-Case "Unknown request type: User-Agent skipped without throwing" {
    # HttpClient is a real .NET type that is neither WebClient nor HttpWebRequest
    $httpClient = New-Object System.Net.Http.HttpClient
    try {
        # Must complete without exception — the unknown-type branch emits a warning and moves on
        Set-C3DRequestHeaders -Request $httpClient -Headers @{ 'User-Agent' = 'C3DUploadTools-PowerShell/1.0' }
    } finally {
        $httpClient.Dispose()
    }
}

# --- Summary ---

Write-Host "`nResults" -ForegroundColor Magenta
Write-Host "=======" -ForegroundColor Magenta
Write-Host "Passed: $testsPassed" -ForegroundColor Green
Write-Host "Failed: $testsFailed" -ForegroundColor $(if ($testsFailed -gt 0) { 'Red' } else { 'Green' })

if ($testsFailed -gt 0) { exit 1 }
Write-Host "`nAll header tests passed." -ForegroundColor Green
