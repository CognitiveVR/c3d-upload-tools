function New-C3DUploadSession {
    <#
    .SYNOPSIS
        Creates a new C3D upload session with configuration and type safety.

    .DESCRIPTION
        Factory function that creates a properly configured upload session using
        the C3D classes for better type safety and validation.

    .PARAMETER ApiKey
        API key for authentication. If not provided, loads from environment.

    .PARAMETER Environment
        Target environment ('prod' or 'dev'). Defaults to 'prod'.

    .PARAMETER SceneId
        Default scene ID for operations. Can be overridden per operation.

    .EXAMPLE
        $session = New-C3DUploadSession
        # Creates session with environment variables

    .EXAMPLE
        $session = New-C3DUploadSession -ApiKey $key -Environment dev
        # Creates session with specific configuration

    .OUTPUTS
        PSCustomObject with Configuration, CreateUploadRequest, and ExecuteRequest methods
    #>

    [CmdletBinding()]
    [OutputType([PSCustomObject])]
    param(
        [Parameter()]
        [string]$ApiKey,

        [Parameter()]
        [ValidateSet('prod', 'dev')]
        [string]$Environment = 'prod',

        [Parameter()]
        [ValidateScript({
            if ([string]::IsNullOrWhiteSpace($_)) { return $true }
            if ($_ -notmatch '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$') {
                throw "Invalid UUID format for SceneId: '$_'"
            }
            $true
        })]
        [string]$SceneId
    )

    Write-C3DLog -Message "Creating new C3D upload session" -Level Info

    try {
        # Create configuration
        $config = if ($ApiKey) {
            [C3DConfiguration]::new($ApiKey, $Environment)
        } else {
            [C3DConfiguration]::FromEnvironment()
            $config.DefaultEnvironment = $Environment
            $config
        }

        if ($SceneId) {
            $config.DefaultSceneId = $SceneId
        }

        # Validate configuration
        if (-not $config.IsValid()) {
            throw "Invalid configuration: API key is required and environment must be valid"
        }

        Write-C3DLog -Message "Session created for environment: $($config.DefaultEnvironment)" -Level Info
        if ($config.DefaultSceneId) {
            Write-C3DLog -Message "Default scene ID: $($config.DefaultSceneId)" -Level Debug
        }

        # Create session object with methods
        $session = [PSCustomObject]@{
            Configuration = $config

            # Method to create upload requests
            CreateSceneUploadRequest = {
                param([string]$SceneDirectory, [string]$SceneId)

                if (-not $SceneId -and $this.Configuration.DefaultSceneId) {
                    $SceneId = $this.Configuration.DefaultSceneId
                }

                $apiUrl = if ($SceneId) {
                    $this.Configuration.GetApiUrl($this.Configuration.DefaultEnvironment, "scenes/$SceneId")
                } else {
                    $this.Configuration.GetApiUrl($this.Configuration.DefaultEnvironment, "scenes")
                }

                $request = [C3DUploadRequest]::new($apiUrl, @{})

                # Add required scene files
                $requiredFiles = @('scene.bin', 'scene.gltf', 'screenshot.png', 'settings.json')
                foreach ($fileName in $requiredFiles) {
                    $filePath = Join-Path $SceneDirectory $fileName
                    $request.AddFile($fileName, $filePath)
                }

                $request.ProgressTitle = "Uploading Scene Files"
                return $request
            }

            CreateObjectUploadRequest = {
                param([string]$SceneId, [string]$ObjectFilename, [string]$ObjectDirectory, [string]$ObjectId)

                if (-not $SceneId -and $this.Configuration.DefaultSceneId) {
                    $SceneId = $this.Configuration.DefaultSceneId
                }

                if (-not $SceneId) {
                    throw "SceneId is required for object upload"
                }

                $apiUrl = if ($ObjectId) {
                    $this.Configuration.GetApiUrl($this.Configuration.DefaultEnvironment, "objects/$SceneId/$ObjectId")
                } else {
                    $this.Configuration.GetApiUrl($this.Configuration.DefaultEnvironment, "objects/$SceneId")
                }

                $request = [C3DUploadRequest]::new($apiUrl, @{})

                # Add required object files
                $gltfFile = Join-Path $ObjectDirectory "$ObjectFilename.gltf"
                $binFile = Join-Path $ObjectDirectory "$ObjectFilename.bin"
                $thumbnailFile = Join-Path $ObjectDirectory "cvr_object_thumbnail.png"

                $request.AddFile("$ObjectFilename.gltf", $gltfFile)
                $request.AddFile("$ObjectFilename.bin", $binFile)
                $request.AddFile("cvr_object_thumbnail.png", $thumbnailFile)

                # Add texture files
                $textureFiles = Get-ChildItem -Path $ObjectDirectory -Filter "*.png" -File | Where-Object { $_.Name -ne "cvr_object_thumbnail.png" }
                foreach ($textureFile in $textureFiles) {
                    $request.AddFile($textureFile.Name, $textureFile.FullName)
                }

                $request.ProgressTitle = "Uploading Object Files"
                return $request
            }

            CreateManifestUploadRequest = {
                param([string]$SceneId, [object]$ManifestData)

                if (-not $SceneId -and $this.Configuration.DefaultSceneId) {
                    $SceneId = $this.Configuration.DefaultSceneId
                }

                if (-not $SceneId) {
                    throw "SceneId is required for manifest upload"
                }

                $apiUrl = $this.Configuration.GetApiUrl($this.Configuration.DefaultEnvironment, "objects/$SceneId")

                $manifestJson = if ($ManifestData -is [string]) {
                    $ManifestData
                } else {
                    $ManifestData | ConvertTo-Json -Depth 10
                }

                $request = [C3DUploadRequest]::new($apiUrl, $manifestJson, 'application/json')
                $request.ProgressTitle = "Uploading Object Manifest"
                return $request
            }

            # Method to execute requests
            ExecuteRequest = {
                param([C3DUploadRequest]$Request, [string]$OperationType = 'Upload')

                Write-C3DLog -Message "Executing $OperationType request to: $($Request.Uri)" -Level Info

                # Validate request
                if (-not $Request.IsValid()) {
                    throw "Invalid upload request: Check that all files exist and URI is valid"
                }

                # Prepare parameters for Invoke-C3DApiRequest
                $params = @{
                    Uri = $Request.Uri
                    Method = $Request.Method
                    ApiKey = $this.Configuration.ApiKey
                    TimeoutSeconds = $Request.TimeoutSeconds
                    ProgressTitle = $Request.ProgressTitle
                }

                # Add request-specific parameters
                if ($Request.Files.Count -gt 0) {
                    $params.FormData = $Request.Files
                } elseif ($Request.Body) {
                    $params.Body = $Request.Body
                    $params.ContentType = $Request.ContentType
                }

                if ($Request.Headers.Count -gt 0) {
                    $params.Headers = $Request.Headers
                }

                # Execute the request
                $httpResponse = Invoke-C3DApiRequest @params

                # Convert to C3DApiResponse
                $apiResponse = [C3DApiResponse]::new($httpResponse.StatusCode, $httpResponse.Body, $httpResponse.TimingMs)
                if (-not $httpResponse.Success -and $httpResponse.Error) {
                    $apiResponse = [C3DApiResponse]::new($httpResponse.StatusCode, $httpResponse.Body, $httpResponse.TimingMs, $httpResponse.Error)
                }
                $apiResponse.Headers = $httpResponse.Headers
                $apiResponse.RawResponse = $httpResponse

                # Create upload result
                $result = [C3DUploadResult]::new($OperationType, $apiResponse)

                # Add file information
                foreach ($fieldName in $Request.Files.Keys) {
                    $result.AddUploadedFile($fieldName, $Request.Files[$fieldName])
                }

                Write-C3DLog -Message "Request completed: $($result.GetSummary())" -Level Info

                return $result
            }
        }

        # Add script methods to the session object
        $session | Add-Member -MemberType ScriptMethod -Name 'CreateSceneUploadRequest' -Value $session.CreateSceneUploadRequest
        $session | Add-Member -MemberType ScriptMethod -Name 'CreateObjectUploadRequest' -Value $session.CreateObjectUploadRequest
        $session | Add-Member -MemberType ScriptMethod -Name 'CreateManifestUploadRequest' -Value $session.CreateManifestUploadRequest
        $session | Add-Member -MemberType ScriptMethod -Name 'ExecuteRequest' -Value $session.ExecuteRequest

        return $session

    } catch {
        Write-C3DLog -Message "Failed to create upload session: $($_.Exception.Message)" -Level Error
        throw
    }
}