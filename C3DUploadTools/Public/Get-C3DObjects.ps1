function Get-C3DObjects {
    <#
    .SYNOPSIS
        Lists objects for a Cognitive3D scene.

    .DESCRIPTION
        PowerShell equivalent of list-objects.sh that retrieves and displays all
        objects associated with a scene from the Cognitive3D API.

    .PARAMETER SceneId
        UUID of the scene to list objects for. Can be provided as parameter or
        set via C3D_SCENE_ID environment variable.

        The scene must exist and contain uploaded objects to return results.

    .PARAMETER Environment
        Target environment for API call. Valid values: 'prod' (default), 'dev'
        - prod: https://data.cognitive3d.com/v0/scenes/{SceneId}
        - dev: https://data.c3ddev.com/v0/scenes/{SceneId}

    .PARAMETER OutputFile
        Optional path to save raw JSON response to file. Supports .json and .txt extensions.
        The parent directory must exist. Useful for:
        - Archiving object data
        - Offline analysis
        - Integration with other tools
        - Backup and audit trails

    .PARAMETER FormatAsManifest
        Format output as object manifest JSON structure and save to
        {SceneId}_object_manifest.json in current directory.

        The manifest includes:
        - Object IDs and names
        - Mesh references
        - Transform data (position, rotation, scale)
        - Custom properties and metadata

    .EXAMPLE
        Get-C3DObjects -SceneId "12345678-1234-1234-1234-123456789012"

        Lists all objects associated with a scene. Returns:
        - Object names and IDs
        - Mesh information and file references
        - Transform data (position, rotation, scale)
        - Upload timestamps and metadata
        - Object status and visibility settings

    .EXAMPLE
        $env:C3D_SCENE_ID = "12345678-1234-1234-1234-123456789012"
        Get-C3DObjects

        Uses environment variable for scene ID. Convenient when working
        with the same scene repeatedly or in batch operations.

    .EXAMPLE
        Get-C3DObjects -SceneId "12345678-1234-1234-1234-123456789012" -Environment dev -OutputFile "objects.json"

        Retrieves objects from development environment and saves raw JSON response
        to file for:
        - Backup and version control
        - Offline analysis
        - Integration with other tools
        - Debugging and troubleshooting

    .EXAMPLE
        Get-C3DObjects -SceneId "12345678-1234-1234-1234-123456789012" -FormatAsManifest

        Formats output as an object manifest and saves to {SceneId}_object_manifest.json.
        Useful for:
        - Creating manifest templates
        - Migrating objects between scenes
        - Backup and restore operations
        - Batch object management

    .EXAMPLE
        # Verify objects after upload
        $uploadedObjects = Get-C3DObjects -SceneId $sceneId
        Write-Host "Scene contains $($uploadedObjects.Count) objects:"
        foreach ($obj in $uploadedObjects) {
            Write-Host "  - $($obj.name) (ID: $($obj.sdkId))"
        }

        Displays a summary of all objects in a scene with names and IDs.
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Position = 0, HelpMessage = "Scene ID (UUID format) or set C3D_SCENE_ID environment variable")]
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
        
        [Parameter(HelpMessage = "Target environment: 'prod' or 'dev'")]
        [ValidateSet('prod', 'dev')]
        [string]$Environment = $(if ($env:C3D_DEFAULT_ENVIRONMENT) { $env:C3D_DEFAULT_ENVIRONMENT } else { 'prod' }),
        
        [Parameter(HelpMessage = "Optional path to save raw JSON response")]
        [ValidateScript({
            if ([string]::IsNullOrWhiteSpace($_)) {
                return $true  # Allow empty
            }
            $parentDir = Split-Path $_ -Parent
            if ($parentDir -and -not (Test-Path $parentDir -PathType Container)) {
                throw "Output directory does not exist: $parentDir"
            }
            $extension = [System.IO.Path]::GetExtension($_)
            if ($extension -notin @('.json', '.txt', '')) {
                throw "Invalid file extension for OutputFile: '$extension'. Use .json or .txt"
            }
            $true
        })]
        [string]$OutputFile,
        
        [Parameter(HelpMessage = "Format output as object manifest JSON structure")]
        [switch]$FormatAsManifest
    )
    
    Write-C3DLog -Message "Starting object list retrieval" -Level Info
    
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
        Test-C3DApiKey
        
        # Get scene details to find latest version
        $sceneUrl = Get-C3DApiUrl -Environment $Environment -Endpoint "scenes/$SceneId"
        Write-C3DLog -Message "Getting scene details from: $sceneUrl" -Level Debug
        
        $sceneResponse = Invoke-C3DApiRequest -Uri $sceneUrl -Method GET
        if ($sceneResponse.StatusCode -ne 200) {
            throw "Failed to get scene info. HTTP $($sceneResponse.StatusCode): $($sceneResponse.Content)"
        }
        
        $sceneData = $sceneResponse.Content | ConvertFrom-Json
        $latestVersion = $sceneData.versions | Sort-Object versionNumber | Select-Object -Last 1
        
        if (-not $latestVersion) {
            throw "Could not extract version ID from scene response"
        }
        
        $versionId = $latestVersion.id
        Write-C3DLog -Message "Resolved latest sceneVersionId: $versionId" -Level Debug
        
        # Get objects for the version
        $objectsUrl = Get-C3DApiUrl -Environment $Environment -Endpoint "versions/$versionId/objects"
        Write-C3DLog -Message "Requesting objects from: $objectsUrl" -Level Debug
        
        $objectsResponse = Invoke-C3DApiRequest -Uri $objectsUrl -Method GET
        if ($objectsResponse.StatusCode -ne 200) {
            throw "Failed to get objects. HTTP $($objectsResponse.StatusCode): $($objectsResponse.Content)"
        }
        
        $objectsData = $objectsResponse.Content | ConvertFrom-Json
        
        # Display results
        Write-Host "Scene Objects:" -ForegroundColor Cyan
        $objectsData | ConvertTo-Json -Depth 10 | Write-Host
        
        # Save raw output if requested
        if ($OutputFile) {
            $objectsData | ConvertTo-Json -Depth 10 | Out-File -FilePath $OutputFile -Encoding UTF8
            Write-C3DLog -Message "Wrote raw output to: $OutputFile" -Level Info
        }
        
        # Save formatted manifest if requested
        if ($FormatAsManifest) {
            $manifestFile = "${SceneId}_object_manifest.json"
            $manifest = @{
                objects = $objectsData | ForEach-Object {
                    @{
                        id = $_.sdkId
                        mesh = $_.meshName
                        name = $_.name
                        scaleCustom = if ($_.scaleCustom) { $_.scaleCustom } else { @(1.0, 1.0, 1.0) }
                        initialPosition = if ($_.initialPosition) { $_.initialPosition } else { @(0.0, 0.0, 0.0) }
                        initialRotation = if ($_.initialRotation) { $_.initialRotation } else { @(0.0, 0.0, 0.0, 1.0) }
                    }
                }
            }
            $manifest | ConvertTo-Json -Depth 10 | Out-File -FilePath $manifestFile -Encoding UTF8
            Write-C3DLog -Message "Wrote formatted manifest to: $manifestFile" -Level Info
        }
        
        return $objectsData
        
    } catch {
        Write-C3DLog -Message "Failed to list objects: $($_.Exception.Message)" -Level Error
        throw
    }
}