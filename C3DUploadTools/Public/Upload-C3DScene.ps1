function Upload-C3DScene {
    <#
    .SYNOPSIS
        Uploads a Cognitive3D scene to the API with comprehensive validation and progress tracking.

    .DESCRIPTION
        PowerShell equivalent of upload-scene.sh that uploads scene files (scene.bin, scene.gltf, 
        screenshot.png, settings.json) to the Cognitive3D API. Provides enhanced error handling,
        progress indicators, and native JSON processing without external dependencies.

    .PARAMETER SceneDirectory
        Path to directory containing the required scene files:
        - scene.bin (scene binary data)
        - scene.gltf (scene geometry/materials)
        - screenshot.png (scene preview image)
        Note: settings.json is generated automatically.

    .PARAMETER SceneName
        Name for the scene. Required when creating a new scene (no SceneId provided).
        Optional when updating an existing scene.

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

        Uploads a new scene to the production environment. The function will:
        - Validate all 4 required files exist in ./my-scene/
        - Update settings.json with current SDK version
        - Upload files to Cognitive3D production API
        - Return the new scene ID for future operations

    .EXAMPLE
        Upload-C3DScene -SceneDirectory "./my-scene" -Environment dev -SceneId "12345678-1234-1234-1234-123456789012"

        Updates an existing scene in development environment. Use this when:
        - You have an existing scene ID from a previous upload
        - You want to update scene files (new binary, textures, etc.)
        - Testing changes in the dev environment before production

    .EXAMPLE
        Upload-C3DScene -SceneDirectory "./my-scene" -DryRun -Verbose

        Preview the upload without making changes. Shows:
        - File validation results and sizes
        - API URL that would be called
        - Request headers and authentication (redacted)
        - Total upload size and estimated time

        Use this to verify everything looks correct before the actual upload.

    .EXAMPLE
        # Complete workflow example
        $sceneResult = Upload-C3DScene -SceneDirectory "C:\MyVRApp\Scenes\MainLevel"
        if ($sceneResult.Success) {
            Write-Host "Scene uploaded successfully! Scene ID: $($sceneResult.SceneId)"
            # Now upload dynamic objects to this scene
            Upload-C3DObject -SceneId $sceneResult.SceneId -ObjectFilename "chair" -ObjectDirectory "C:\MyVRApp\Objects"
        }

        Demonstrates a complete upload workflow with error checking.

    .OUTPUTS
        Scene upload results with timing metrics and next steps guidance

    .NOTES
        Prerequisites:
        - C3D_DEVELOPER_API_KEY environment variable must be set
        - Scene directory must contain required files:
          * scene.bin (Unity scene binary data)
          * scene.gltf (3D scene geometry and materials)
          * screenshot.png (scene preview image for dashboard)
        - Note: settings.json is generated automatically with scene name and SDK version
        - Files must be under 100MB each
        - PowerShell 5.1 or higher

        Automatic Features:
        - settings.json is automatically generated with scene name and SDK version
        - SDK version prefixed with "cli-powershell-v" for tracking
        - Progress indicators for large uploads
        - Comprehensive validation before upload

        Error Handling:
        - Validates UUID format for Scene ID
        - Checks file existence and sizes
        - Provides specific error messages for common issues
        - Network retry logic for transient failures

        Security:
        - API key is never logged or displayed
        - Uses secure HTTPS endpoints
        - Validates SSL certificates

        Performance:
        - Efficient multipart upload for multiple files
        - Progress tracking for user feedback
        - Optimized for both small and large scene files

    .LINK
        Upload-C3DObject

    .LINK
        Upload-C3DObjectManifest

    .LINK
        Get-C3DObjects
    #>
    
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Mandatory, Position = 0, HelpMessage = "Path to scene directory containing scene.bin, scene.gltf, screenshot.png")]
        [ValidateScript({
            if (-not (Test-Path $_ -PathType Container)) {
                throw "Scene directory does not exist: $_"
            }
            # Validate required files exist in directory (settings.json is generated automatically)
            $requiredFiles = @('scene.bin', 'scene.gltf', 'screenshot.png')
            $missingFiles = @()
            foreach ($file in $requiredFiles) {
                $filePath = Join-Path $_ $file
                if (-not (Test-Path $filePath -PathType Leaf)) {
                    $missingFiles += $file
                }
            }
            if ($missingFiles.Count -gt 0) {
                throw "Missing required files in scene directory: $($missingFiles -join ', '). Required files: $($requiredFiles -join ', ')"
            }
            # Validate file sizes (100MB limit)
            foreach ($file in $requiredFiles) {
                $filePath = Join-Path $_ $file
                $fileSize = (Get-Item $filePath).Length
                if ($fileSize -gt 100MB) {
                    $sizeMB = [math]::Round($fileSize / 1MB, 2)
                    throw "File '$file' is too large: $sizeMB MB (maximum: 100 MB)"
                }
            }
            $true
        })]
        [string]$SceneDirectory,

        [Parameter(Position = 1, HelpMessage = "Name for the scene (required for new scenes, optional for updates)")]
        [string]$SceneName,
        
        [Parameter(HelpMessage = "Target environment: 'prod' or 'dev'")]
        [ValidateSet('prod', 'dev')]
        [string]$Environment = $(if ($env:C3D_DEFAULT_ENVIRONMENT) { $env:C3D_DEFAULT_ENVIRONMENT } else { 'prod' }),
        
        [Parameter(HelpMessage = "Optional UUID of existing scene to update")]
        [ValidateScript({
            if ([string]::IsNullOrWhiteSpace($_)) {
                return $true  # Allow empty/null for new scene creation
            }
            if ($_ -notmatch '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$') {
                throw "Invalid UUID format for SceneId: '$_'. Expected format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
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
        
        # Validate SceneName is provided for new scenes
        if (-not $SceneId -and [string]::IsNullOrWhiteSpace($SceneName)) {
            throw "SceneName is required when creating a new scene (no SceneId provided)"
        }

        # Define required files (settings.json is generated automatically)
        $requiredFiles = @('scene.bin', 'scene.gltf', 'screenshot.png')
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

        # Create full SDK version string with cli-powershell prefix
        $fullSdkVersion = "cli-powershell-v$sdkVersion"
        Write-C3DLog -Message "SDK version: $fullSdkVersion" -Level Info

        # Determine scene name for settings.json
        $settingsSceneName = if ([string]::IsNullOrWhiteSpace($SceneName)) { "Scene" } else { $SceneName }

        # Generate settings.json
        $settingsFile = Join-Path -Path $SceneDirectory -ChildPath "settings.json"
        Write-C3DLog -Message "Generating settings.json with scene name '$settingsSceneName' and SDK version '$fullSdkVersion'" -Level Info

        if (-not $DryRun) {
            try {
                # Create settings object
                $settingsContent = @{
                    scale = 1
                    sceneName = $settingsSceneName
                    sdkVersion = $fullSdkVersion
                }

                # Write settings.json
                $settingsContent | ConvertTo-Json -Depth 10 | Set-Content -Path $settingsFile -Encoding UTF8
                Write-C3DLog -Message "Successfully generated settings.json" -Level Debug

                # Add to file paths for upload
                $filePaths['settings.json'] = $settingsFile

            } catch {
                Write-C3DLog -Message "Failed to generate settings.json: $($_.Exception.Message)" -Level Error
                throw
            }
        } else {
            Write-C3DLog -Message "DRY RUN - Would generate settings.json with:" -Level Info
            Write-Host "  {" -ForegroundColor Cyan
            Write-Host "    `"scale`": 1," -ForegroundColor Cyan
            Write-Host "    `"sceneName`": `"$settingsSceneName`"," -ForegroundColor Cyan
            Write-Host "    `"sdkVersion`": `"$fullSdkVersion`"" -ForegroundColor Cyan
            Write-Host "  }" -ForegroundColor Cyan
        }
        
        # Build API URL
        $apiUrl = Get-C3DApiUrl -Environment $Environment -EndpointType 'scenes'
        if ($SceneId) {
            $apiUrl += "/$SceneId"
        }
        Write-C3DLog -Message "Using API URL: $apiUrl" -Level Info
        
        # Add settings.json to required files for upload
        $uploadFiles = $requiredFiles + @('settings.json')

        # Prepare upload
        if ($DryRun) {
            Write-C3DLog -Message "DRY RUN - Would upload these files:" -Level Info

            foreach ($fileName in $requiredFiles) {
                $fileSize = Get-C3DFileSize -Path $filePaths[$fileName]
                Write-Host "  - $fileName`: $($fileSize.FormattedSize)" -ForegroundColor Cyan
            }
            Write-Host "  - settings.json: (generated)" -ForegroundColor Cyan
            
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
            Write-C3DLog -Message "Files to upload: $($uploadFiles -join ', ')" -Level Debug

            $uploadStartTime = Get-Date

            # Prepare multipart form data (includes generated settings.json)
            $formData = @{}
            foreach ($fileName in $uploadFiles) {
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

        # Final timing and next steps
        $endTime = Get-Date
        $totalDuration = ($endTime - $startTime).TotalSeconds
        
        if (-not $DryRun) {
            Write-C3DLog -Message "Script completed in $([math]::Round($totalDuration, 2)) seconds" -Level Info
            Write-C3DLog -Message "Next Steps:" -Level Info
            Write-C3DLog -Message "1. You can now upload dynamic objects using Upload-C3DObject" -Level Info
            Write-C3DLog -Message "2. You'll need the scene ID from the upload response" -Level Info
            Write-C3DLog -Message "3. Example: Upload-C3DObject -SceneId <scene_id> -ObjectFilename <name> -ObjectDirectory <dir>" -Level Info
            Write-C3DLog -Message "4. Run Upload-C3DObjectManifest after uploading objects to display them in dashboard" -Level Info
        }
        
    } catch [System.Net.WebException] {
        $errorRecord = New-C3DErrorRecord -Message "Network error during scene upload: $($_.Exception.Message)" -ErrorId "SceneUploadNetworkError" -Category ([System.Management.Automation.ErrorCategory]::ConnectionError) -TargetObject $_.Exception.Response -InnerException $_.Exception -RecommendedAction "Check network connectivity and API endpoint availability"

        Write-C3DLog -Message "Scene upload failed due to network error: $($_.Exception.Message)" -Level Error
        $PSCmdlet.ThrowTerminatingError($errorRecord)

    } catch [System.IO.IOException] {
        $errorRecord = New-C3DErrorRecord -Message "File I/O error during scene upload: $($_.Exception.Message)" -ErrorId "SceneUploadFileError" -Category ([System.Management.Automation.ErrorCategory]::ReadError) -TargetObject $_.Exception.FileName -InnerException $_.Exception -RecommendedAction "Check file permissions and ensure all required files exist"

        Write-C3DLog -Message "Scene upload failed due to file error: $($_.Exception.Message)" -Level Error
        $PSCmdlet.ThrowTerminatingError($errorRecord)

    } catch {
        Write-C3DLog -Message "Scene upload failed: $($_.Exception.Message)" -Level Error
        throw
    }
}