function Invoke-C3DApiRequest {
    <#
    .SYNOPSIS
        Makes HTTP requests to the Cognitive3D API with enhanced error handling.

    .DESCRIPTION
        Simplified main API request function that delegates to specialized helper functions.
        Provides comprehensive error handling, timing metrics, and response validation.
        Supports both JSON and multipart form data requests.

    .PARAMETER Uri
        The complete API endpoint URI.

    .PARAMETER Method
        HTTP method: GET, POST, PUT, DELETE, etc.

    .PARAMETER Body
        The request body (for POST/PUT requests). Can be string, hashtable, or PSObject.

    .PARAMETER ContentType
        The Content-Type header. Defaults to 'application/json'.

    .PARAMETER Headers
        Additional HTTP headers as a hashtable.

    .PARAMETER FilePath
        Path to file for upload (multipart requests).

    .PARAMETER FieldName
        Form field name for file uploads.

    .PARAMETER FormData
        Hashtable of form field names to file paths for multipart uploads.
        Example: @{ 'scene.bin' = '/path/to/scene.bin'; 'scene.gltf' = '/path/to/scene.gltf' }

    .PARAMETER ApiKey
        API key for authentication. If not provided, will use Get-C3DApiKey.

    .PARAMETER TimeoutSeconds
        Request timeout in seconds. Defaults to 300 (5 minutes).

    .PARAMETER ProgressTitle
        Title for progress indicator during large uploads.

    .EXAMPLE
        $response = Invoke-C3DApiRequest -Uri "https://api.cognitive3d.com/v0/scenes" -Method GET

    .EXAMPLE
        $body = @{ name = "test-scene"; version = "1.0" } | ConvertTo-Json
        $response = Invoke-C3DApiRequest -Uri $apiUrl -Method POST -Body $body

    .OUTPUTS
        PSCustomObject with properties: StatusCode, Body, Headers, TimingMs, Success
    #>

    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [uri]$Uri,

        [Parameter()]
        [Microsoft.PowerShell.Commands.WebRequestMethod]$Method = 'GET',

        [Parameter()]
        [object]$Body,

        [Parameter()]
        [string]$ContentType = 'application/json',

        [Parameter()]
        [hashtable]$Headers = @{},

        [Parameter()]
        [ValidateScript({Test-Path $_ -PathType Leaf})]
        [string]$FilePath,

        [Parameter()]
        [string]$FieldName = 'file',

        [Parameter()]
        [hashtable]$FormData,

        [Parameter()]
        [string]$ApiKey,

        [Parameter()]
        [int]$TimeoutSeconds = 300,

        [Parameter()]
        [string]$ProgressTitle = 'API Request'
    )

    $startTime = Get-Date
    Write-C3DLog -Message "Starting $Method request to: $Uri" -Level Info

    try {
        # Validate and get API key
        if (-not $ApiKey) {
            $ApiKey = Get-C3DApiKey
        }

        # Prepare request headers
        $requestHeaders = $Headers.Clone()
        $requestHeaders['User-Agent'] = 'C3DUploadTools-PowerShell/1.0'

        # Handle different request types
        $httpResponse = $null
        $requestBody = $null
        $requestContentType = $ContentType

        if ($FormData) {
            # Multiple file upload using multipart form data
            Write-C3DLog -Message "Preparing multipart form data upload with $($FormData.Count) files" -Level Info

            # Show progress for large requests
            Write-Progress -Activity $ProgressTitle -Status "Preparing multipart data..." -PercentComplete 10

            $formDataResult = New-C3DMultipartFormData -FormData $FormData
            $requestBody = $formDataResult.Body
            $requestContentType = $formDataResult.ContentType

            Write-Progress -Activity $ProgressTitle -Status "Uploading files..." -PercentComplete 50

            # Use WebClient for multipart uploads (Windows compatible)
            $httpResponse = Send-C3DHttpRequest -Uri $Uri -Method $Method -Body $requestBody -ContentType $requestContentType -ApiKey $ApiKey -Headers $requestHeaders -TimeoutSeconds $TimeoutSeconds -UseWebClient

        } elseif ($FilePath) {
            # Single file upload using multipart form data
            Write-C3DLog -Message "Preparing single file upload: $FilePath" -Level Info

            Write-Progress -Activity $ProgressTitle -Status "Preparing file data..." -PercentComplete 10

            $formDataResult = New-C3DSingleFileFormData -FilePath $FilePath -FieldName $FieldName
            $requestBody = $formDataResult.Body
            $requestContentType = $formDataResult.ContentType

            Write-Progress -Activity $ProgressTitle -Status "Uploading file..." -PercentComplete 50

            # Use WebClient for file uploads
            $httpResponse = Send-C3DHttpRequest -Uri $Uri -Method $Method -Body $requestBody -ContentType $requestContentType -ApiKey $ApiKey -Headers $requestHeaders -TimeoutSeconds $TimeoutSeconds -UseWebClient

        } else {
            # Standard JSON or text request
            Write-C3DLog -Message "Preparing standard HTTP request" -Level Debug

            if ($Body) {
                $requestBody = if ($Body -is [string]) { $Body } else { $Body | ConvertTo-Json -Depth 10 }
                Write-C3DLog -Message "Request body prepared ($($requestBody.Length) characters)" -Level Debug
            }

            # Show progress for large requests
            if ($Body -and $requestBody.Length -gt 10000) {
                Write-Progress -Activity $ProgressTitle -Status "Sending request..." -PercentComplete 25
            }

            # Use standard WebRequest for non-multipart requests
            $httpResponse = Send-C3DHttpRequest -Uri $Uri -Method $Method -Body $requestBody -ContentType $requestContentType -ApiKey $ApiKey -Headers $requestHeaders -TimeoutSeconds $TimeoutSeconds
        }

        # Calculate timing
        $endTime = Get-Date
        $timingMs = [math]::Round(($endTime - $startTime).TotalMilliseconds, 0)

        # Clear progress
        Write-Progress -Activity $ProgressTitle -Completed

        # Convert to standardized API response
        $apiResponse = ConvertTo-C3DApiResponse -HttpResponse $httpResponse -TimingMs $timingMs

        Write-C3DLog -Message "Request completed in ${timingMs}ms (HTTP $($apiResponse.StatusCode))" -Level Info

        return $apiResponse

    } catch [System.Net.WebException] {
        $endTime = Get-Date
        $timingMs = [math]::Round(($endTime - $startTime).TotalMilliseconds, 0)

        Write-Progress -Activity $ProgressTitle -Completed
        Write-C3DLog -Message "Network error after ${timingMs}ms: $($_.Exception.Message)" -Level Error

        # Create error response
        $errorResponse = [PSCustomObject]@{
            StatusCode = if ($_.Exception.Response) { [int]$_.Exception.Response.StatusCode } else { $null }
            Content = if ($_.Exception.Response) {
                try {
                    $stream = $_.Exception.Response.GetResponseStream()
                    $reader = New-Object System.IO.StreamReader($stream)
                    $content = $reader.ReadToEnd()
                    $reader.Close()
                    $stream.Close()
                    $content
                } catch { "Unable to read response body" }
            } else { $null }
            Headers = @{}
            StatusDescription = if ($_.Exception.Response) { $_.Exception.Response.StatusDescription } else { $null }
        }

        return ConvertTo-C3DApiResponse -HttpResponse $errorResponse -TimingMs $timingMs -ErrorMessage $_.Exception.Message

    } catch {
        $endTime = Get-Date
        $timingMs = [math]::Round(($endTime - $startTime).TotalMilliseconds, 0)

        Write-Progress -Activity $ProgressTitle -Completed
        Write-C3DLog -Message "Request failed after ${timingMs}ms: $($_.Exception.Message)" -Level Error

        # Create generic error response
        $errorResponse = [PSCustomObject]@{
            StatusCode = $null
            Content = $null
            Headers = @{}
            StatusDescription = $null
        }

        return ConvertTo-C3DApiResponse -HttpResponse $errorResponse -TimingMs $timingMs -ErrorMessage $_.Exception.Message
    }
}