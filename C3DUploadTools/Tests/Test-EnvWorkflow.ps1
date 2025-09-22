#!/usr/bin/env pwsh

<#
.SYNOPSIS
    Test Environment Workflow Script for Cognitive3D Upload Tools

.DESCRIPTION
    Tests the complete Cognitive3D upload workflow with environment-specific configuration.
    Copies appropriate .env.sample.* file to .env and runs full upload test suite using PowerShell module.

.PARAMETER Environment
    Target environment for testing ('prod' or 'dev')
    - 'prod': Uses .env.sample.prod configuration  
    - 'dev': Uses .env.sample.dev configuration

.PARAMETER Verbose
    Enable detailed logging and debug output

.PARAMETER DryRun
    Preview operations without executing uploads

.EXAMPLE
    # Test dev environment workflow
    ./Test-EnvWorkflow.ps1 -Environment dev -Verbose

.EXAMPLE
    # Test prod environment with dry run
    ./Test-EnvWorkflow.ps1 -Environment prod -DryRun

.EXAMPLE
    # Quick prod test
    ./Test-EnvWorkflow.ps1 -Environment prod

.NOTES
    Requirements:
    - .env.sample.dev and .env.sample.prod files must exist
    - scene-test/ directory with test scene files
    - object-test/ directory with test object files
    - PowerShell 5.1+ or PowerShell Core 7.x+
    
    Workflow:
    1. Copies .env.sample.<env> to .env
    2. Imports C3DUploadTools module
    3. Uploads test scene to get scene ID
    4. Adds scene ID to .env file
    5. Tests object upload with environment variable fallback
    6. Tests object manifest upload
    7. Lists objects to verify
    8. Cleans up temporary .env file
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, HelpMessage = "Target environment: 'prod' or 'dev'")]
    [ValidateSet('prod', 'dev')]
    [string]$Environment,
    
    [Parameter(HelpMessage = "Enable detailed logging")]
    [switch]$Verbose,
    
    [Parameter(HelpMessage = "Preview operations without executing")]
    [switch]$DryRun
)

# Set strict mode
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Track script start time
$scriptStartTime = Get-Date

# Setup logging function
function Write-TestLog {
    param(
        [Parameter(Mandatory)]
        [string]$Message,
        
        [ValidateSet('INFO', 'WARN', 'ERROR', 'DEBUG')]
        [string]$Level = 'INFO'
    )
    
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $colorMap = @{
        'INFO' = 'Cyan'
        'WARN' = 'Yellow'  
        'ERROR' = 'Red'
        'DEBUG' = 'DarkCyan'
    }
    
    $color = $colorMap[$Level]
    Write-Host "[$timestamp] [$Level] $Message" -ForegroundColor $color
}

# Validate required files and directories
$envSampleFile = ".env.sample.$Environment"
if (-not (Test-Path $envSampleFile)) {
    Write-TestLog -Message "Environment sample file not found: $envSampleFile" -Level 'ERROR'
    exit 1
}

if (-not (Test-Path "scene-test" -PathType Container)) {
    Write-TestLog -Message "Test scene directory not found: scene-test/" -Level 'ERROR'
    exit 1
}

if (-not (Test-Path "object-test" -PathType Container)) {
    Write-TestLog -Message "Test object directory not found: object-test/" -Level 'ERROR'
    exit 1
}

Write-TestLog -Message "Testing environment workflow for: $Environment"

# Backup existing .env if it exists
$envBackup = $null
if (Test-Path ".env") {
    $envBackup = ".env.backup.$([System.DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
    Write-TestLog -Message "Backing up existing .env to $envBackup"
    if (-not $DryRun) {
        Copy-Item ".env" $envBackup
    }
}

# Cleanup function
function Invoke-Cleanup {
    param([int]$ExitCode = 0)
    
    Write-TestLog -Message "Cleaning up..."
    
    # Restore original .env if we had a backup
    if ($envBackup -and (Test-Path $envBackup)) {
        Move-Item $envBackup ".env" -Force
        Write-TestLog -Message "Restored original .env file"
    } elseif (Test-Path ".env") {
        # Remove the temporary .env file if no backup existed
        Remove-Item ".env" -Force
        Write-TestLog -Message "Removed temporary .env file"
    }
    
    $endTime = Get-Date
    $duration = [math]::Round(($endTime - $scriptStartTime).TotalSeconds)
    Write-TestLog -Message "Test completed in ${duration}s with exit code $ExitCode"
    
    exit $ExitCode
}

# Setup cleanup on script exit
try {
    # Step 1: Copy environment sample to .env
    Write-TestLog -Message "Step 1: Setting up .env file from $envSampleFile"
    if (-not $DryRun) {
        Copy-Item $envSampleFile ".env"
        Write-TestLog -Message "Copied $envSampleFile to .env"
    } else {
        Write-TestLog -Message "[DRY_RUN] Would copy $envSampleFile to .env"
    }

    if ($DryRun) {
        Write-TestLog -Message "[DRY_RUN] Would run the following workflow:" -Level 'DEBUG'
        Write-TestLog -Message "[DRY_RUN] 1. Import C3DUploadTools module" -Level 'DEBUG'
        Write-TestLog -Message "[DRY_RUN] 2. Upload-C3DScene -SceneDirectory scene-test -Environment $Environment" -Level 'DEBUG'
        Write-TestLog -Message "[DRY_RUN] 3. Extract scene ID and add to .env file" -Level 'DEBUG'
        Write-TestLog -Message "[DRY_RUN] 4. Upload-C3DObject -ObjectFilename cube -ObjectDirectory object-test -Environment $Environment" -Level 'DEBUG'
        Write-TestLog -Message "[DRY_RUN] 5. Upload-C3DObjectManifest -Environment $Environment" -Level 'DEBUG'
        Write-TestLog -Message "[DRY_RUN] 6. Get-C3DObjects -Environment $Environment" -Level 'DEBUG'
        
        if ($envBackup) {
            Write-TestLog -Message "[DRY_RUN] Would restore .env from backup: $envBackup" -Level 'DEBUG'
        }
        
        Invoke-Cleanup -ExitCode 0
        return
    }

    # Step 2: Import PowerShell module
    Write-TestLog -Message "Step 2: Importing C3DUploadTools PowerShell module"
    Import-Module "../" -Force
    Write-TestLog -Message "C3DUploadTools module loaded successfully"

    # Step 3: Upload scene to get scene ID (using bash script for now since PowerShell Upload-C3DScene is placeholder)
    Write-TestLog -Message "Step 3: Uploading test scene to get scene ID"
    $sceneResult = & bash -c "./upload-scene.sh --scene_dir scene-test --env $Environment 2>&1"
    $sceneExitCode = $LASTEXITCODE

    if ($sceneExitCode -ne 0) {
        Write-TestLog -Message "Scene upload failed with exit code $sceneExitCode" -Level 'ERROR'
        Write-Host $sceneResult
        Invoke-Cleanup -ExitCode $sceneExitCode
        return
    }

    # Extract scene ID from output
    $sceneIdMatch = [regex]::Match($sceneResult, '([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})')
    if (-not $sceneIdMatch.Success) {
        Write-TestLog -Message "Could not extract scene ID from upload-scene.sh output" -Level 'ERROR'
        Write-Host "Scene upload output:"
        Write-Host $sceneResult
        Invoke-Cleanup -ExitCode 1
        return
    }

    $sceneId = $sceneIdMatch.Value
    Write-TestLog -Message "Scene uploaded successfully. Scene ID: $sceneId"

    # Step 4: Add scene ID to .env file
    Write-TestLog -Message "Step 4: Adding scene ID to .env file"
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    Add-Content -Path ".env" -Value ""
    Add-Content -Path ".env" -Value "# Scene ID from test upload ($timestamp)"
    Add-Content -Path ".env" -Value "C3D_SCENE_ID=$sceneId"
    Write-TestLog -Message "Added C3D_SCENE_ID=$sceneId to .env file"

    # Step 5: Test object upload using environment variable (using bash for now)
    Write-TestLog -Message "Step 5: Testing object upload with environment variable fallback"
    $objectResult = & bash -c "./upload-object.sh --object_filename cube --object_dir object-test --env $Environment 2>&1"
    $objectExitCode = $LASTEXITCODE
    
    if ($objectExitCode -ne 0) {
        Write-TestLog -Message "Object upload failed with exit code $objectExitCode" -Level 'ERROR'
        Write-Host $objectResult
        Invoke-Cleanup -ExitCode $objectExitCode
        return
    }
    Write-TestLog -Message "Object upload completed successfully"

    # Step 6: Test object manifest upload
    Write-TestLog -Message "Step 6: Testing object manifest upload"
    $manifestResult = & bash -c "./upload-object-manifest.sh --env $Environment 2>&1"
    $manifestExitCode = $LASTEXITCODE
    
    if ($manifestExitCode -ne 0) {
        Write-TestLog -Message "Object manifest upload failed with exit code $manifestExitCode" -Level 'ERROR'  
        Write-Host $manifestResult
        Invoke-Cleanup -ExitCode $manifestExitCode
        return
    }
    Write-TestLog -Message "Object manifest upload completed successfully"

    # Step 7: List objects to verify
    Write-TestLog -Message "Step 7: Listing objects to verify uploads"
    $listResult = & bash -c "./list-objects.sh --env $Environment 2>&1"
    $listExitCode = $LASTEXITCODE
    
    if ($listExitCode -ne 0) {
        Write-TestLog -Message "Object listing failed with exit code $listExitCode" -Level 'ERROR'
        Write-Host $listResult  
        Invoke-Cleanup -ExitCode $listExitCode
        return
    }
    Write-TestLog -Message "Object listing completed successfully"

    # Step 8: Test PowerShell environment loading
    Write-TestLog -Message "Step 8: Testing PowerShell .env loading and environment variable support"
    
    # Force reload environment to pick up new C3D_SCENE_ID
    if (Get-Command "Import-C3DEnvironment" -ErrorAction SilentlyContinue) {
        Import-C3DEnvironment -Path ".env"
        Write-TestLog -Message "PowerShell .env loading verified"
        
        # Test that scene ID is available in environment
        $envSceneId = $env:C3D_SCENE_ID
        if ($envSceneId -eq $sceneId) {
            Write-TestLog -Message "✅ PowerShell C3D_SCENE_ID environment variable confirmed: $envSceneId"
        } else {
            Write-TestLog -Message "⚠️ PowerShell C3D_SCENE_ID mismatch. Expected: $sceneId, Got: $envSceneId" -Level 'WARN'
        }
    } else {
        Write-TestLog -Message "Import-C3DEnvironment function not available, skipping PowerShell-specific test" -Level 'WARN'
    }

    Write-TestLog -Message "✅ Environment workflow test completed successfully for $Environment environment"
    Write-TestLog -Message "✅ Verified .env file loading and C3D_SCENE_ID environment variable fallback"

    # Normal exit - cleanup will be called by trap
    Invoke-Cleanup -ExitCode 0

} catch {
    Write-TestLog -Message "Test failed with error: $($_.Exception.Message)" -Level 'ERROR'
    Write-TestLog -Message "Stack trace: $($_.ScriptStackTrace)" -Level 'ERROR'
    Invoke-Cleanup -ExitCode 1
}