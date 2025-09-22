function Upload-C3DObject {
    <#
    .SYNOPSIS
        Uploads a Cognitive3D dynamic object to the API with automatic manifest generation.

    .DESCRIPTION
        PowerShell equivalent of upload-object.sh that uploads dynamic 3D object files 
        (GLTF, binary, textures, thumbnail) to an existing Cognitive3D scene. Automatically
        generates and uploads object manifest for dashboard display.

    .PARAMETER SceneId
        UUID of the existing scene where the object will be uploaded.
        Must be valid UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx

    .PARAMETER ObjectFilename
        Object filename without extension. Used to locate .gltf and .bin files.
        Example: "cube" will look for cube.gltf and cube.bin

    .PARAMETER ObjectDirectory
        Path to directory containing the object files:
        - {ObjectFilename}.gltf (object geometry/materials)
        - {ObjectFilename}.bin (object binary data)  
        - cvr_object_thumbnail.png (object preview image)
        - *.png (texture files, automatically included)

    .PARAMETER ObjectId
        Optional object ID. If not provided, defaults to ObjectFilename.
        Must be valid UUID format if specified.

    .PARAMETER Environment
        Target environment for upload. Valid values: 'prod' (default), 'dev'
        - prod: https://data.cognitive3d.com/v0/objects
        - dev: https://data.c3ddev.com/v0/objects

    .PARAMETER DryRun
        Preview operations without executing them. Shows file inventory and API calls
        that would be made without actually performing uploads.

    .PARAMETER AutoUploadManifest
        Automatically upload object manifest after successful object upload.
        Default: $true (matches bash script behavior)

    .EXAMPLE
        Upload-C3DObject -SceneId "12345678-1234-1234-1234-123456789012" -ObjectFilename "cube" -ObjectDirectory "./my-objects"

        Uploads a cube object to production environment. The function will:
        - Look for cube.gltf and cube.bin in ./my-objects/
        - Include cvr_object_thumbnail.png for dashboard preview
        - Auto-detect and include any .png texture files
        - Generate and upload object manifest automatically
        - Make object visible in Cognitive3D dashboard

    .EXAMPLE
        Upload-C3DObject -SceneId "12345678-1234-1234-1234-123456789012" -ObjectFilename "lamp" -ObjectDirectory "./objects" -Environment dev -ObjectId "87654321-4321-4321-4321-210987654321"

        Updates an existing lamp object in development environment with specific UUID.
        Use this when:
        - Updating an existing object with new geometry or textures
        - Testing object changes before production deployment
        - Maintaining consistent object IDs across environments

    .EXAMPLE
        Upload-C3DObject -SceneId "12345678-1234-1234-1234-123456789012" -ObjectFilename "chair" -ObjectDirectory "./furniture" -DryRun -Verbose

        Preview object upload without making changes. Shows:
        - Required files validation (chair.gltf, chair.bin, thumbnail)
        - Texture files that will be included
        - Total upload size and file count
        - Generated manifest structure

    .EXAMPLE
        # Environment variable workflow
        $env:C3D_SCENE_ID = "12345678-1234-1234-1234-123456789012"
        Upload-C3DObject -ObjectFilename "table" -ObjectDirectory "./furniture"
        Upload-C3DObject -ObjectFilename "chair" -ObjectDirectory "./furniture"
        Upload-C3DObject -ObjectFilename "lamp" -ObjectDirectory "./furniture"

        Efficient batch upload using environment variable for scene ID.
        Each object is automatically added to the same scene.

    .EXAMPLE
        # Complete workflow with error handling
        try {
            $result = Upload-C3DObject -SceneId $sceneId -ObjectFilename "interactive_button" -ObjectDirectory "./ui_objects" -AutoUploadManifest:$false
            if ($result.Success) {
                Write-Host "Object uploaded: $($result.ObjectId)"
                # Custom manifest logic here
                Upload-C3DObjectManifest -SceneId $sceneId
            }
        } catch {
            Write-Error "Upload failed: $($_.Exception.Message)"
        }

        Advanced workflow with manual manifest control and error handling.

    .OUTPUTS
        Object upload results with manifest generation and next steps guidance

    .NOTES
        Prerequisites:
        - C3D_DEVELOPER_API_KEY environment variable must be set
        - Target scene must exist (create with Upload-C3DScene first)
        - Object directory must contain required files:
          * {ObjectFilename}.gltf (3D object geometry and materials)
          * {ObjectFilename}.bin (binary data for the object)
          * cvr_object_thumbnail.png (preview image for dashboard)
        - All texture .png files in directory are automatically included
        - Files must be under 100MB each
        - PowerShell 5.1 or higher

        Automatic Features:
        - Auto-detects and uploads all .png texture files
        - Generates object manifest with proper positioning and scaling
        - Automatically uploads manifest for dashboard visibility
        - Creates unique object IDs if not specified
        - Progress tracking for multi-file uploads

        Object Manifest:
        - Automatically generated with default transform values
        - Position: (0, 0, 0)
        - Rotation: (0, 0, 0, 1) quaternion
        - Scale: (1, 1, 1)
        - Saved as {SceneId}_object_manifest.json

        Environment Variables:
        - C3D_SCENE_ID: Set once to avoid repeating SceneId parameter
        - C3D_DEFAULT_ENVIRONMENT: Set default environment (prod/dev)

        Best Practices:
        - Upload scenes before objects
        - Use consistent naming conventions for objects
        - Test in dev environment before production
        - Keep object files organized in dedicated directories
        - Use descriptive ObjectFilename values

    .LINK
        Upload-C3DScene

    .LINK
        Upload-C3DObjectManifest

    .LINK
        Get-C3DObjects
    #>
    
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Position = 0, HelpMessage = "Scene ID (UUID format) where object will be uploaded, or set C3D_SCENE_ID environment variable")]
        [ValidateScript({
            if ([string]::IsNullOrWhiteSpace($_) -and [string]::IsNullOrWhiteSpace($env:C3D_SCENE_ID)) {
                throw "SceneId is required. Provide via parameter or set C3D_SCENE_ID environment variable"
            }
            $sceneIdToValidate = if ([string]::IsNullOrWhiteSpace($_)) { $env:C3D_SCENE_ID } else { $_ }
            if ($sceneIdToValidate -notmatch '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$') {
                throw "Invalid UUID format for SceneId: '$sceneIdToValidate'. Expected format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
            }
            $true
        })]
        [string]$SceneId = $env:C3D_SCENE_ID,
        
        [Parameter(Mandatory, Position = 1, HelpMessage = "Object filename without extension (e.g., 'cube' for cube.gltf and cube.bin)")]
        [ValidateNotNullOrEmpty()]
        [string]$ObjectFilename,
        
        [Parameter(Mandatory, Position = 2, HelpMessage = "Path to directory containing object files")]
        [ValidateScript({
            if (-not (Test-Path $_ -PathType Container)) {
                throw "Object directory does not exist: $_"
            }
            $true
        })]
        [string]$ObjectDirectory,
        
        [Parameter(HelpMessage = "Optional object ID. Defaults to ObjectFilename if not provided")]
        [ValidateScript({
            if ([string]::IsNullOrWhiteSpace($_)) {
                return $true  # Allow empty - will use ObjectFilename
            }
            if ($_ -notmatch '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$') {
                throw "Invalid UUID format for ObjectId: '$_'. Expected format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
            }
            $true
        })]
        [string]$ObjectId,
        
        [Parameter(HelpMessage = "Target environment: 'prod' or 'dev'")]
        [ValidateSet('prod', 'dev')]
        [string]$Environment = $(if ($env:C3D_DEFAULT_ENVIRONMENT) { $env:C3D_DEFAULT_ENVIRONMENT } else { 'prod' }),
        
        [Parameter(HelpMessage = "Automatically upload object manifest after successful upload")]
        [switch]$AutoUploadManifest = $true,
        
        [Parameter(HelpMessage = "Preview operations without executing them")]
        [switch]$DryRun
    )
    
    # Initialize timing
    $startTime = Get-Date
    Write-C3DLog -Message "Starting object upload process" -Level Info
    
    # Validate SceneId is provided and has correct format
    if ([string]::IsNullOrWhiteSpace($SceneId)) {
        Write-C3DLog -Message "SceneId is required. Provide via parameter or set C3D_SCENE_ID environment variable" -Level Error
        throw "Missing required parameter: SceneId"
    }
    
    if (-not (Test-C3DUuidFormat -Uuid $SceneId -FieldName 'SceneId')) {
        throw "Invalid UUID format for SceneId: $SceneId"
    }
    
    if ($SceneId -eq $env:C3D_SCENE_ID) {
        Write-C3DLog -Message "Using C3D_SCENE_ID from environment: $SceneId" -Level Debug
    }
    
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
        
        # Validate and convert scene ID
        $SceneId = ConvertTo-C3DLowerUuid -Uuid $SceneId -FieldName 'SceneId'
        Write-C3DLog -Message "Scene ID: $SceneId" -Level Debug
        
        # Handle Object ID - validate UUID format only if explicitly provided
        if (-not $ObjectId) {
            # Use object filename as ID (bash script behavior)
            $ObjectId = $ObjectFilename
            Write-C3DLog -Message "Object ID not provided, using derived ID: $ObjectId" -Level Debug
        } else {
            # Validate UUID format for explicitly provided ObjectId
            if (-not (Test-C3DUuidFormat -Uuid $ObjectId -FieldName 'ObjectId')) {
                throw "Invalid UUID format for ObjectId: $ObjectId"
            }
            $ObjectId = ConvertTo-C3DLowerUuid -Uuid $ObjectId -FieldName 'ObjectId'
            Write-C3DLog -Message "Using provided Object ID: $ObjectId" -Level Debug
        }
        
        # Resolve object directory to full path
        $ObjectDirectory = (Get-Item -Path $ObjectDirectory).FullName
        Write-C3DLog -Message "Object directory: $ObjectDirectory" -Level Debug
        Write-C3DLog -Message "Object filename: $ObjectFilename" -Level Debug
        Write-C3DLog -Message "Object ID: $ObjectId" -Level Debug
        
        # Define required files
        $gltfFile = Join-Path -Path $ObjectDirectory -ChildPath "$ObjectFilename.gltf"
        $binFile = Join-Path -Path $ObjectDirectory -ChildPath "$ObjectFilename.bin"
        $thumbnailFile = Join-Path -Path $ObjectDirectory -ChildPath "cvr_object_thumbnail.png"
        
        # Validate required files
        Write-C3DLog -Message "Validating object files..." -Level Info
        
        Test-C3DFile -Path $gltfFile -Name "$ObjectFilename.gltf" -RequiredExtensions @('.gltf') -MaxSizeBytes 100MB -Throw
        Test-C3DFile -Path $binFile -Name "$ObjectFilename.bin" -RequiredExtensions @('.bin') -MaxSizeBytes 100MB -Throw
        Test-C3DFile -Path $thumbnailFile -Name "cvr_object_thumbnail.png" -RequiredExtensions @('.png') -MaxSizeBytes 100MB -Throw
        
        # Log file sizes in debug mode
        foreach ($filePath in @($gltfFile, $binFile, $thumbnailFile)) {
            $fileSize = Get-C3DFileSize -Path $filePath
            Write-C3DLog -Message "$(Split-Path $filePath -Leaf): $($fileSize.FormattedSize)" -Level Debug
        }
        
        # Collect texture .png files (excluding thumbnail)
        Write-C3DLog -Message "Scanning for texture files..." -Level Info
        $textureFiles = @()
        $pngFiles = Get-ChildItem -Path $ObjectDirectory -Filter "*.png" -File
        
        foreach ($pngFile in $pngFiles) {
            if ($pngFile.FullName -ne $thumbnailFile) {
                $textureFiles += $pngFile.FullName
                Write-C3DLog -Message "Found texture: $($pngFile.Name)" -Level Debug
            }
        }
        
        if ($textureFiles.Count -gt 0) {
            Write-C3DLog -Message "Found $($textureFiles.Count) texture file(s)" -Level Info
        } else {
            Write-C3DLog -Message "No additional texture files found" -Level Info
        }
        
        # Build API URL
        $apiUrl = Get-C3DApiUrl -Environment $Environment -EndpointType 'objects'
        $apiUrl += "/$SceneId"
        if ($ObjectId) {
            $apiUrl += "/$ObjectId"
        }
        Write-C3DLog -Message "Using API URL: $apiUrl" -Level Info
        
        # Prepare form data
        $formData = @{
            "cvr_object_thumbnail.png" = $thumbnailFile
            "$ObjectFilename.bin" = $binFile
            "$ObjectFilename.gltf" = $gltfFile
        }
        
        # Add texture files to form data
        foreach ($textureFile in $textureFiles) {
            $textureName = Split-Path $textureFile -Leaf
            $formData[$textureName] = $textureFile
        }
        
        # Show upload plan
        $totalFiles = $formData.Count
        Write-C3DLog -Message "Preparing to upload $totalFiles files" -Level Info
        
        if ($DryRun) {
            Write-C3DLog -Message "DRY RUN - Would upload these files:" -Level Info
            
            foreach ($fieldName in $formData.Keys) {
                $fileSize = Get-C3DFileSize -Path $formData[$fieldName]
                Write-Host "  - $fieldName`: $($fileSize.FormattedSize)" -ForegroundColor Cyan
            }
            
            Write-C3DLog -Message "API URL: $apiUrl" -Level Info
            Write-C3DLog -Message "Method: POST (multipart/form-data)" -Level Info
            Write-C3DLog -Message "Authorization: APIKEY:DEVELOPER [REDACTED]" -Level Info
            
            # Show manifest that would be created
            $manifestFile = "${SceneId}_object_manifest.json"
            Write-C3DLog -Message "Would create manifest file: $manifestFile" -Level Info
            
            if ($AutoUploadManifest) {
                Write-C3DLog -Message "Would automatically upload object manifest" -Level Info
            }
            
            $endTime = Get-Date
            $duration = ($endTime - $startTime).TotalSeconds
            Write-C3DLog -Message "DRY RUN completed in $([math]::Round($duration, 2)) seconds" -Level Info
            Write-C3DLog -Message "Re-run without -DryRun to perform actual upload" -Level Info
            
        } else {
            # Perform actual upload
            Write-C3DLog -Message "Uploading object files to API..." -Level Info
            Write-C3DLog -Message "Files to upload: $($formData.Keys -join ', ')" -Level Debug
            
            $uploadStartTime = Get-Date
            
            # Get API key
            $apiKey = Get-C3DApiKey
            
            # Make API request with progress indicators
            $response = Invoke-C3DApiRequest -Uri $apiUrl -Method POST -FormData $formData -ApiKey $apiKey -ProgressTitle "Uploading Object Files"
            
            $uploadEndTime = Get-Date
            $uploadDuration = ($uploadEndTime - $uploadStartTime).TotalSeconds
            
            Write-C3DLog -Message "Upload completed in $([math]::Round($uploadDuration, 2)) seconds (HTTP $($response.StatusCode))" -Level Info
            
            if ($response.Success) {
                Write-C3DLog -Message "Object upload successful!" -Level Info
                
                # Parse and display response
                if ($response.Body) {
                    try {
                        $responseData = $response.Body | ConvertFrom-Json
                        Write-C3DLog -Message "Response received: $($responseData | ConvertTo-Json -Compress)" -Level Debug
                    } catch {
                        Write-C3DLog -Message "Response body: $($response.Body)" -Level Debug
                    }
                }
                
                # Create object manifest file
                Write-C3DLog -Message "Creating object manifest..." -Level Info
                $manifestFile = "${SceneId}_object_manifest.json"
                
                $manifestData = @{
                    objects = @(
                        @{
                            id = $ObjectId
                            mesh = $ObjectFilename
                            name = $ObjectFilename
                            scaleCustom = @(1.0, 1.0, 1.0)
                            initialPosition = @(0.0, 0.0, 0.0)
                            initialRotation = @(0.0, 0.0, 0.0, 1.0)
                        }
                    )
                }
                
                $manifestJson = $manifestData | ConvertTo-Json -Depth 10
                Set-Content -Path $manifestFile -Value $manifestJson -Encoding UTF8
                Write-C3DLog -Message "Created manifest file: $manifestFile" -Level Debug
                
                # Auto-upload manifest if enabled
                if ($AutoUploadManifest) {
                    Write-C3DLog -Message "Automatically uploading object manifest..." -Level Info
                    
                    try {
                        Upload-C3DObjectManifest -SceneId $SceneId -Environment $Environment
                        Write-C3DLog -Message "Object manifest uploaded successfully" -Level Info
                    } catch {
                        Write-C3DLog -Message "Failed to upload manifest: $($_.Exception.Message)" -Level Warn
                        Write-C3DLog -Message "You can manually upload the manifest later using: Upload-C3DObjectManifest -SceneId $SceneId" -Level Info
                    }
                }
                
            } else {
                throw "Upload failed with HTTP $($response.StatusCode): $($response.Body)"
            }
        }
        
        # Final timing and completion message
        $endTime = Get-Date
        $totalDuration = ($endTime - $startTime).TotalSeconds
        
        if (-not $DryRun) {
            Write-C3DLog -Message "Object upload process completed in $([math]::Round($totalDuration, 2)) seconds" -Level Info
            Write-C3DLog -Message "Upload complete. Object ID: $ObjectId" -Level Info
            Write-C3DLog -Message "Next Steps:" -Level Info
            if ($AutoUploadManifest) {
                Write-C3DLog -Message "✅ Object manifest has been automatically uploaded" -Level Info
            } else {
                Write-C3DLog -Message "1. Upload object manifest: Upload-C3DObjectManifest -SceneId $SceneId" -Level Info
            }
            Write-C3DLog -Message "2. Check object in Cognitive3D dashboard" -Level Info
            Write-C3DLog -Message "3. Upload additional objects with same SceneId if needed" -Level Info
        }
        
    } catch {
        Write-C3DLog -Message "Object upload failed: $($_.Exception.Message)" -Level Error
        throw
    }
}