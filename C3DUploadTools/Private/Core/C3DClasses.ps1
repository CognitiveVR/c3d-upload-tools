# Simplified PowerShell classes for better compatibility

class C3DConfiguration {
    [string] $ApiKey
    [string] $DefaultEnvironment = 'prod'
    [string] $DefaultSceneId
    [hashtable] $EnvironmentUrls = @{
        'prod' = 'https://data.cognitive3d.com/v0'
        'dev' = 'https://data.c3ddev.com/v0'
    }
    [int] $DefaultTimeoutSeconds = 300
    [string] $UserAgent = 'C3DUploadTools-PowerShell/1.0'

    # Constructor with API key
    C3DConfiguration([string] $apiKey) {
        $this.ApiKey = $apiKey
    }

    # Constructor with API key and environment
    C3DConfiguration([string] $apiKey, [string] $environment) {
        $this.ApiKey = $apiKey
        $this.DefaultEnvironment = $environment
    }

    # Get API URL for endpoint
    [string] GetApiUrl([string] $environment, [string] $endpoint) {
        if (-not $this.EnvironmentUrls.ContainsKey($environment)) {
            throw "Unknown environment: $environment. Valid values: $($this.EnvironmentUrls.Keys -join ', ')"
        }

        $baseUrl = $this.EnvironmentUrls[$environment]
        return "$baseUrl/$endpoint"
    }

    # Validate configuration
    [bool] IsValid() {
        return (-not [string]::IsNullOrWhiteSpace($this.ApiKey)) -and $this.EnvironmentUrls.ContainsKey($this.DefaultEnvironment)
    }
}

class C3DUploadRequest {
    [string] $Uri
    [string] $Method = 'POST'
    [hashtable] $Files = @{}
    [hashtable] $Headers = @{}
    [string] $ContentType
    [object] $Body
    [int] $TimeoutSeconds = 300
    [string] $ProgressTitle = 'API Request'

    # Constructor for file upload
    C3DUploadRequest([string] $uri, [hashtable] $files) {
        $this.Uri = $uri
        $this.Files = $files
        $this.Method = 'POST'
    }

    # Constructor for JSON request
    C3DUploadRequest([string] $uri, [object] $body, [string] $contentType) {
        $this.Uri = $uri
        $this.Body = $body
        $this.ContentType = $contentType
        $this.Method = 'POST'
    }

    # Add file to upload
    [void] AddFile([string] $fieldName, [string] $filePath) {
        if (-not (Test-Path $filePath -PathType Leaf)) {
            throw "File not found: $filePath"
        }
        $this.Files[$fieldName] = $filePath
    }

    # Add header
    [void] AddHeader([string] $name, [string] $value) {
        $this.Headers[$name] = $value
    }

    # Get total file size
    [long] GetTotalFileSize() {
        $totalSize = 0
        foreach ($filePath in $this.Files.Values) {
            if (Test-Path $filePath -PathType Leaf) {
                $totalSize += (Get-Item $filePath).Length
            }
        }
        return $totalSize
    }

    # Validate request
    [bool] IsValid() {
        # Check URI
        if ([string]::IsNullOrWhiteSpace($this.Uri)) {
            return $false
        }

        # Validate files exist
        foreach ($filePath in $this.Files.Values) {
            if (-not (Test-Path $filePath -PathType Leaf)) {
                return $false
            }
        }

        return $true
    }

    # Get request type
    [string] GetRequestType() {
        if ($this.Files.Count -gt 0) {
            return 'FileUpload'
        } elseif ($this.Body) {
            return 'JsonRequest'
        } else {
            return 'SimpleRequest'
        }
    }
}

class C3DApiResponse {
    [int] $StatusCode
    [string] $Body
    [hashtable] $Headers = @{}
    [int] $TimingMs
    [bool] $Success
    [string] $Error
    [object] $RawResponse
    [datetime] $Timestamp = [datetime]::UtcNow

    # Constructor for successful response
    C3DApiResponse([int] $statusCode, [string] $body, [int] $timingMs) {
        $this.StatusCode = $statusCode
        $this.Body = $body
        $this.TimingMs = $timingMs
        $this.Success = $statusCode -ge 200 -and $statusCode -lt 300
    }

    # Constructor for error response
    C3DApiResponse([int] $statusCode, [string] $body, [int] $timingMs, [string] $error) {
        $this.StatusCode = $statusCode
        $this.Body = $body
        $this.TimingMs = $timingMs
        $this.Error = $error
        $this.Success = $false
    }

    # Get parsed JSON body
    [object] GetJsonBody() {
        if ([string]::IsNullOrWhiteSpace($this.Body)) {
            return $null
        }

        try {
            return $this.Body | ConvertFrom-Json
        } catch {
            throw "Response body is not valid JSON: $($_.Exception.Message)"
        }
    }

    # Check if response indicates authentication error
    [bool] IsAuthenticationError() {
        return $this.StatusCode -eq 401
    }

    # Check if response indicates not found
    [bool] IsNotFound() {
        return $this.StatusCode -eq 404
    }

    # Check if response indicates rate limiting
    [bool] IsRateLimited() {
        return $this.StatusCode -eq 429
    }

    # Check if response indicates server error
    [bool] IsServerError() {
        return $this.StatusCode -ge 500 -and $this.StatusCode -lt 600
    }

    # Convert to string representation
    [string] ToString() {
        $status = if ($this.Success) { 'SUCCESS' } else { 'ERROR' }
        return "[$status] HTTP $($this.StatusCode) - $($this.TimingMs)ms"
    }
}

class C3DUploadResult {
    [string] $OperationType
    [string] $SceneId
    [string] $ObjectId
    [C3DApiResponse] $Response
    [hashtable] $UploadedFiles = @{}
    [long] $TotalBytesUploaded = 0
    [int] $TotalDurationMs
    [string[]] $NextSteps = @()
    [datetime] $Timestamp = [datetime]::UtcNow

    # Constructor
    C3DUploadResult([string] $operationType, [C3DApiResponse] $response) {
        $this.OperationType = $operationType
        $this.Response = $response
        $this.TotalDurationMs = $response.TimingMs
    }

    # Add uploaded file info
    [void] AddUploadedFile([string] $fieldName, [string] $filePath) {
        if (Test-Path $filePath -PathType Leaf) {
            $fileInfo = Get-Item $filePath
            $this.UploadedFiles[$fieldName] = @{
                FilePath = $fileInfo.FullName
                FileName = $fileInfo.Name
                Size = $fileInfo.Length
                SizeFormatted = if ($fileInfo.Length -gt 1MB) {
                    "$([math]::Round($fileInfo.Length / 1MB, 2)) MB"
                } elseif ($fileInfo.Length -gt 1KB) {
                    "$([math]::Round($fileInfo.Length / 1KB, 2)) KB"
                } else {
                    "$($fileInfo.Length) bytes"
                }
            }
            $this.TotalBytesUploaded += $fileInfo.Length
        }
    }

    # Add next step
    [void] AddNextStep([string] $step) {
        $this.NextSteps += $step
    }

    # Check if upload was successful
    [bool] IsSuccessful() {
        return $this.Response.Success
    }

    # Get formatted summary
    [string] GetSummary() {
        $status = if ($this.IsSuccessful()) { 'SUCCESS' } else { 'FAILED' }
        $files = if ($this.UploadedFiles.Count -gt 0) { "$($this.UploadedFiles.Count) files" } else { 'no files' }
        $size = if ($this.TotalBytesUploaded -gt 0) {
            if ($this.TotalBytesUploaded -gt 1MB) {
                "$([math]::Round($this.TotalBytesUploaded / 1MB, 2)) MB"
            } else {
                "$([math]::Round($this.TotalBytesUploaded / 1KB, 2)) KB"
            }
        } else { '0 bytes' }

        $durationSec = [math]::Round($this.TotalDurationMs / 1000, 2)
        return "$($this.OperationType) [$status] - $files ($size) in ${durationSec}s"
    }

    # Convert to string representation
    [string] ToString() {
        return $this.GetSummary()
    }
}

# Factory functions for creating configuration
function New-C3DConfiguration {
    [CmdletBinding()]
    param(
        [Parameter()]
        [string]$ApiKey,

        [Parameter()]
        [ValidateSet('prod', 'dev')]
        [string]$Environment = 'prod'
    )

    if (-not $ApiKey) {
        $ApiKey = [System.Environment]::GetEnvironmentVariable('C3D_DEVELOPER_API_KEY')
        if (-not $ApiKey) {
            throw 'C3D_DEVELOPER_API_KEY environment variable is required'
        }
    }

    $config = [C3DConfiguration]::new($ApiKey, $Environment)

    # Load optional environment variables
    $defaultSceneId = [System.Environment]::GetEnvironmentVariable('C3D_SCENE_ID')
    if ($defaultSceneId) {
        $config.DefaultSceneId = $defaultSceneId
    }

    return $config
}