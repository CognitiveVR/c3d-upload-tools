function Upload-C3DScene {
    <#
    .SYNOPSIS
        Uploads a Cognitive3D scene to the API with comprehensive validation and progress tracking.

    .DESCRIPTION
        PowerShell equivalent of upload-scene.sh that uploads scene files (scene.bin, scene.gltf, 
        screenshot.png, settings.json) to the Cognitive3D API. Provides enhanced error handling,
        progress indicators, and native JSON processing without external dependencies.

    .PARAMETER SceneDirectory
        Path to directory containing the 4 required scene files:
        - scene.bin (scene binary data)
        - scene.gltf (scene geometry/materials)
        - screenshot.png (scene preview image)
        - settings.json (scene configuration)

    .PARAMETER Environment
        Target environment for upload. Valid values: 'prod' (default), 'dev'
        - prod: https://data.cognitive3d.com/v0/scenes
        - dev: https://data.c3ddev.com/v0/scenes

    .PARAMETER SceneId
        Optional UUID of existing scene to update. If not provided, creates new scene.
        Must be valid UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx

    .PARAMETER DryRun
        Preview operations without executing them. Shows file inventory and API calls
        that would be made without actually performing uploads.

    .EXAMPLE
        Upload-C3DScene -SceneDirectory "./my-scene"
        Uploads scene to production environment (creates new scene)

    .EXAMPLE  
        Upload-C3DScene -SceneDirectory "./my-scene" -Environment dev -SceneId "12345678-1234-1234-1234-123456789012"
        Updates existing scene in development environment

    .EXAMPLE
        Upload-C3DScene -SceneDirectory "./my-scene" -DryRun -Verbose
        Preview upload operations with detailed logging

    .OUTPUTS
        Scene upload results with timing metrics and next steps guidance

    .NOTES
        Prerequisites:
        - C3D_DEVELOPER_API_KEY environment variable must be set
        - Scene directory must contain all 4 required files
        - Files must be under 100MB each
        - settings.json is automatically updated with SDK version from sdk-version.txt
        
        This function creates automatic backups of settings.json with rollback on failure.
    #>
    
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Mandatory, Position = 0, HelpMessage = "Path to scene directory containing scene.bin, scene.gltf, screenshot.png, settings.json")]
        [ValidateScript({
            if (-not (Test-Path $_ -PathType Container)) {
                throw "Scene directory does not exist: $_"
            }
            $true
        })]
        [string]$SceneDirectory,
        
        [Parameter(HelpMessage = "Target environment: 'prod' or 'dev'")]
        [ValidateSet('prod', 'dev')]
        [string]$Environment = 'prod',
        
        [Parameter(HelpMessage = "Optional UUID of existing scene to update")]
        [ValidateScript({
            if ($_ -and -not (Test-C3DUuidFormat -Uuid $_ -FieldName 'SceneId')) {
                throw "Invalid UUID format for SceneId: $_"
            }
            $true
        })]
        [string]$SceneId,
        
        [Parameter(HelpMessage = "Preview operations without executing them")]
        [switch]$DryRun
    )
    
    # Initialize timing
    $startTime = Get-Date
    Write-C3DLog -Message "Starting scene upload process" -Level Info
    
    # Enable debug logging if -Verbose is specified
    if ($PSBoundParameters.ContainsKey('Verbose') -and $PSBoundParameters['Verbose']) {
        $script:VerboseMode = $true
    }
    
    try {
        # Validate prerequisites
        Write-C3DLog -Message "Validating prerequisites..." -Level Info
        
        # Test API key
        Test-C3DApiKey -Throw
        
        # Test environment
        Test-C3DEnvironment -Environment $Environment -Throw
        Write-C3DLog -Message "Using environment: $Environment" -Level Info
        
        # Validate and convert scene ID if provided
        if ($SceneId) {
            $SceneId = ConvertTo-C3DLowerUuid -Uuid $SceneId -FieldName 'SceneId'
            Write-C3DLog -Message "Using scene ID: $SceneId" -Level Info
        }
        
        # Resolve scene directory to full path
        $SceneDirectory = (Get-Item -Path $SceneDirectory).FullName
        Write-C3DLog -Message "Scene directory: $SceneDirectory" -Level Debug
        
        # Define required files
        $requiredFiles = @('scene.bin', 'scene.gltf', 'screenshot.png', 'settings.json')
        $filePaths = @{}
        
        # Validate scene directory and required files
        Test-C3DDirectory -Path $SceneDirectory -Name "scene directory" -RequiredFiles $requiredFiles -Throw
        
        # Build file paths and validate each file
        Write-C3DLog -Message "Validating scene files..." -Level Info
        foreach ($fileName in $requiredFiles) {
            $filePath = Join-Path -Path $SceneDirectory -ChildPath $fileName
            $filePaths[$fileName] = $filePath
            
            # Validate file with 100MB limit
            Test-C3DFile -Path $filePath -Name $fileName -MaxSizeBytes 100MB -Throw
            
            # Log file size in debug mode
            $fileSize = Get-C3DFileSize -Path $filePath
            Write-C3DLog -Message "$fileName`: $($fileSize.FormattedSize)" -Level Debug
        }
        
        # Read and validate SDK version
        $sdkVersionFile = Join-Path -Path $PSScriptRoot -ChildPath "../../sdk-version.txt"
        if (-not (Test-Path -Path $sdkVersionFile -PathType Leaf)) {
            throw "sdk-version.txt not found at: $sdkVersionFile"
        }
        
        $sdkVersion = (Get-Content -Path $sdkVersionFile -Raw).Trim()
        if ([string]::IsNullOrEmpty($sdkVersion)) {
            throw "sdk-version.txt is empty"
        }
        
        # Validate semantic version format (x.y.z)
        if ($sdkVersion -notmatch '^\d+\.\d+\.\d+$') {
            throw "Invalid SDK version format: $sdkVersion. Expected format: x.y.z"
        }
        
        Write-C3DLog -Message "Read SDK version: $sdkVersion" -Level Info
        
        # Backup and update settings.json
        Write-C3DLog -Message "Updating settings.json with SDK version: $sdkVersion" -Level Info
        
        $settingsFile = $filePaths['settings.json']
        
        if (-not $DryRun) {
            # Create backup
            $backupFile = Backup-C3DFile -Path $settingsFile
            
            try {
                # Read current settings.json
                $settingsContent = Get-Content -Path $settingsFile -Raw | ConvertFrom-Json
                
                # Update SDK version
                $settingsContent.sdkVersion = $sdkVersion
                
                # Write updated settings back to file
                $settingsContent | ConvertTo-Json -Depth 10 | Set-Content -Path $settingsFile -Encoding UTF8
                Write-C3DLog -Message "Successfully updated settings.json with SDK version" -Level Debug
                
            } catch {
                Write-C3DLog -Message "Failed to update settings.json: $($_.Exception.Message)" -Level Error
                
                # Attempt rollback
                if ($backupFile -and (Test-Path -Path $backupFile)) {
                    try {
                        Copy-Item -Path $backupFile -Destination $settingsFile -Force
                        Write-C3DLog -Message "Successfully restored settings.json from backup" -Level Info
                    } catch {
                        Write-C3DLog -Message "Failed to restore backup - settings.json may be corrupted" -Level Error
                    }
                }
                throw
            }
        } else {
            Write-C3DLog -Message "DRY RUN - Would update settings.json with SDK version: $sdkVersion" -Level Info
        }
        
        # Build API URL
        $apiUrl = Get-C3DApiUrl -Environment $Environment -EndpointType 'scenes'
        if ($SceneId) {
            $apiUrl += "/$SceneId"
        }
        Write-C3DLog -Message "Using API URL: $apiUrl" -Level Info
        
        # Prepare upload
        if ($DryRun) {
            Write-C3DLog -Message "DRY RUN - Would upload these files:" -Level Info
            
            foreach ($fileName in $requiredFiles) {
                $fileSize = Get-C3DFileSize -Path $filePaths[$fileName]
                Write-Host "  - $fileName`: $($fileSize.FormattedSize)" -ForegroundColor Cyan
            }
            
            Write-C3DLog -Message "API URL: $apiUrl" -Level Info
            Write-C3DLog -Message "Method: POST (multipart/form-data)" -Level Info
            Write-C3DLog -Message "Authorization: APIKEY:DEVELOPER [REDACTED]" -Level Info
            
            $endTime = Get-Date
            $duration = ($endTime - $startTime).TotalSeconds
            Write-C3DLog -Message "DRY RUN completed in $([math]::Round($duration, 2)) seconds" -Level Info
            Write-C3DLog -Message "Re-run without -DryRun to perform actual upload" -Level Info
            
        } else {
            # Perform actual upload
            Write-C3DLog -Message "Uploading scene files to API..." -Level Info
            Write-C3DLog -Message "Files to upload: $($requiredFiles -join ', ')" -Level Debug
            
            $uploadStartTime = Get-Date
            
            # Prepare multipart form data
            $formData = @{}
            foreach ($fileName in $requiredFiles) {
                $formData[$fileName] = $filePaths[$fileName]
            }
            
            # Get API key
            $apiKey = Get-C3DApiKey
            
            # Make API request with progress indicators
            $response = Invoke-C3DApiRequest -Uri $apiUrl -Method POST -FormData $formData -ApiKey $apiKey -ProgressTitle "Uploading Scene Files"
            
            $uploadEndTime = Get-Date
            $uploadDuration = ($uploadEndTime - $uploadStartTime).TotalSeconds
            
            Write-C3DLog -Message "Upload completed in $([math]::Round($uploadDuration, 2)) seconds (HTTP $($response.StatusCode))" -Level Info
            
            if ($response.Success) {
                Write-C3DLog -Message "Scene upload successful!" -Level Info
                
                # Parse and display response
                if ($response.Body) {
                    try {
                        $responseData = $response.Body | ConvertFrom-Json
                        Write-C3DLog -Message "Response received: $($responseData | ConvertTo-Json -Compress)" -Level Debug
                    } catch {
                        Write-C3DLog -Message "Response body: $($response.Body)" -Level Debug
                    }
                }
            } else {
                throw "Upload failed with HTTP $($response.StatusCode): $($response.Body)"
            }
        }
        
        # Clean up backup file
        if (Get-Variable -Name 'backupFile' -Scope Local -ErrorAction SilentlyContinue) {
            if ($backupFile -and (Test-Path -Path $backupFile) -and -not $DryRun) {
                Remove-Item -Path $backupFile -Force
                Write-C3DLog -Message "Cleaned up backup file" -Level Debug
            }
        }
        
        # Final timing and next steps
        $endTime = Get-Date
        $totalDuration = ($endTime - $startTime).TotalSeconds
        
        if (-not $DryRun) {
            Write-C3DLog -Message "Script completed in $([math]::Round($totalDuration, 2)) seconds" -Level Info
            Write-C3DLog -Message "" -Level Info
            Write-C3DLog -Message "Next Steps:" -Level Info
            Write-C3DLog -Message "1. You can now upload dynamic objects using Upload-C3DObject" -Level Info
            Write-C3DLog -Message "2. You'll need the scene ID from the upload response" -Level Info
            Write-C3DLog -Message "3. Example: Upload-C3DObject -SceneId <scene_id> -ObjectFilename <name> -ObjectDirectory <dir>" -Level Info
            Write-C3DLog -Message "4. Run Upload-C3DObjectManifest after uploading objects to display them in dashboard" -Level Info
        }
        
    } catch {
        Write-C3DLog -Message "Scene upload failed: $($_.Exception.Message)" -Level Error
        
        # Clean up backup file on error
        if (Get-Variable -Name 'backupFile' -Scope Local -ErrorAction SilentlyContinue) {
            if ($backupFile -and (Test-Path -Path $backupFile) -and -not $DryRun) {
                try {
                    Remove-Item -Path $backupFile -Force
                    Write-C3DLog -Message "Cleaned up backup file after error" -Level Debug
                } catch {
                    Write-C3DLog -Message "Warning: Could not clean up backup file: $backupFile" -Level Warn
                }
            }
        }
        
        throw
    }
}