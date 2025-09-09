function Upload-C3DObjectManifest {
    <#
    .SYNOPSIS
        Uploads object manifest for a Cognitive3D scene to display objects in dashboard.

    .DESCRIPTION
        PowerShell equivalent of upload-object-manifest.sh that uploads a JSON manifest 
        file containing object definitions for a scene. This enables objects to appear 
        in the Cognitive3D dashboard.

    .PARAMETER SceneId
        UUID of the scene to upload manifest for. Can be provided as parameter or 
        set via C3D_SCENE_ID environment variable.

    .PARAMETER Environment
        Target environment for upload. Valid values: 'prod' (default), 'dev'

    .PARAMETER DryRun
        Preview operations without executing them.

    .EXAMPLE
        Upload-C3DObjectManifest -SceneId "12345678-1234-1234-1234-123456789012"
        Uploads manifest using scene ID parameter

    .EXAMPLE  
        $env:C3D_SCENE_ID = "12345678-1234-1234-1234-123456789012"
        Upload-C3DObjectManifest
        Uploads manifest using environment variable
    #>
    
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Position = 0, HelpMessage = "Scene ID (UUID format) or set C3D_SCENE_ID environment variable")]
        [string]$SceneId = $env:C3D_SCENE_ID,
        
        [Parameter(HelpMessage = "Target environment: 'prod' or 'dev'")]
        [ValidateSet('prod', 'dev')]
        [string]$Environment = $(if ($env:C3D_DEFAULT_ENVIRONMENT) { $env:C3D_DEFAULT_ENVIRONMENT } else { 'prod' }),
        
        [Parameter(HelpMessage = "Preview operations without executing them")]
        [switch]$DryRun
    )
    
    # Initialize timing
    $startTime = Get-Date
    Write-C3DLog -Message "Starting object manifest upload process" -Level Info
    
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
    
    try {
        # Validate prerequisites
        Write-C3DLog -Message "Validating prerequisites..." -Level Info
        Test-C3DApiKey
        
        # Look for manifest file
        $manifestFile = "${SceneId}_object_manifest.json"
        if (-not (Test-C3DFile -Path $manifestFile -MaxSizeBytes 10MB)) {
            throw "Manifest file not found: $manifestFile (run Upload-C3DObject first)"
        }
        
        Write-C3DLog -Message "Found manifest file: $manifestFile" -Level Info
        
        # Get API URL
        $apiUrl = Get-C3DApiUrl -Environment $Environment -Endpoint "objects/$SceneId"
        Write-C3DLog -Message "Upload URL: $apiUrl" -Level Debug
        
        if ($DryRun) {
            Write-C3DLog -Message "DRY RUN - Would upload manifest to: $apiUrl" -Level Info
            Write-C3DLog -Message "File that would be uploaded: $manifestFile" -Level Info
            Write-C3DLog -Message "DRY RUN completed" -Level Info
            return
        }
        
        # Upload manifest
        Write-C3DLog -Message "Uploading object manifest..." -Level Info
        $uploadStartTime = Get-Date
        
        $manifestContent = Get-Content $manifestFile -Raw
        $response = Invoke-C3DApiRequest -Uri $apiUrl -Method POST -Body $manifestContent -ContentType 'application/json'
        
        $uploadEndTime = Get-Date
        $uploadDuration = ($uploadEndTime - $uploadStartTime).TotalSeconds
        Write-C3DLog -Message "Upload completed in $($uploadDuration) seconds (HTTP $($response.StatusCode))" -Level Info
        
        if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
            Write-C3DLog -Message "Object manifest uploaded successfully" -Level Info
            if ($response.Content) {
                $responseObj = $response.Content | ConvertFrom-Json -ErrorAction SilentlyContinue
                if ($responseObj) {
                    Write-C3DLog -Message "Response: $($responseObj | ConvertTo-Json -Compress)" -Level Debug
                }
            }
        } else {
            throw "Upload failed with HTTP $($response.StatusCode): $($response.Content)"
        }
        
        # Log execution time
        $endTime = Get-Date
        $totalDuration = ($endTime - $startTime).TotalSeconds
        Write-C3DLog -Message "Object manifest upload process completed in $($totalDuration) seconds" -Level Info
        
    } catch {
        Write-C3DLog -Message "Object manifest upload failed: $($_.Exception.Message)" -Level Error
        throw
    }
}