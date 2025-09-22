function Send-C3DHttpRequest {
    <#
    .SYNOPSIS
        Sends HTTP requests with proper authentication and error handling.

    .DESCRIPTION
        Handles the actual HTTP request sending with C3D API authentication.
        Supports both multipart and JSON requests with proper error handling.

    .PARAMETER Uri
        The complete API endpoint URI.

    .PARAMETER Method
        HTTP method: GET, POST, PUT, DELETE, etc.

    .PARAMETER Body
        The request body as byte array or string.

    .PARAMETER ContentType
        The Content-Type header.

    .PARAMETER ApiKey
        API key for authentication.

    .PARAMETER Headers
        Additional HTTP headers as a hashtable.

    .PARAMETER TimeoutSeconds
        Request timeout in seconds. Defaults to 300 (5 minutes).

    .PARAMETER UseWebClient
        Use System.Net.WebClient instead of WebRequest for multipart uploads.

    .EXAMPLE
        $response = Send-C3DHttpRequest -Uri $apiUrl -Method POST -Body $bodyBytes -ContentType 'multipart/form-data' -ApiKey $apiKey

    .OUTPUTS
        PSCustomObject with StatusCode, Content, Headers, and StatusDescription properties
    #>

    [CmdletBinding()]
    [OutputType([PSCustomObject])]
    param(
        [Parameter(Mandatory)]
        [uri]$Uri,

        [Parameter()]
        [Microsoft.PowerShell.Commands.WebRequestMethod]$Method = 'GET',

        [Parameter()]
        [object]$Body,

        [Parameter()]
        [string]$ContentType = 'application/json',

        [Parameter(Mandatory)]
        [string]$ApiKey,

        [Parameter()]
        [hashtable]$Headers = @{},

        [Parameter()]
        [int]$TimeoutSeconds = 300,

        [Parameter()]
        [switch]$UseWebClient
    )

    Write-C3DLog -Message "Sending $Method request to: $Uri" -Level Debug
    Write-C3DLog -Message "Content-Type: $ContentType" -Level Debug

    if ($UseWebClient) {
        # Use WebClient for multipart uploads (Windows compatible, no curl dependency)
        Write-C3DLog -Message "Using System.Net.WebClient for request" -Level Debug

        try {
            $webClient = New-Object System.Net.WebClient
            $webClient.Headers.Add('Authorization', "APIKEY:DEVELOPER $ApiKey")
            $webClient.Headers.Add('User-Agent', 'C3DUploadTools-PowerShell/1.0')
            $webClient.Headers.Add('Content-Type', $ContentType)

            # Add additional headers
            foreach ($headerName in $Headers.Keys) {
                $webClient.Headers.Add($headerName, $Headers[$headerName])
            }

            # Perform the upload
            Write-C3DLog -Message "Uploading $($Body.Length) bytes via WebClient" -Level Debug
            $responseBytes = $webClient.UploadData($Uri, $Method.ToString().ToUpper(), $Body)
            $responseContent = [System.Text.Encoding]::UTF8.GetString($responseBytes)

            # WebClient doesn't provide status code directly for successful uploads
            # If we get here without exception, assume success (200)
            $webResponse = [PSCustomObject]@{
                StatusCode = 200
                Content = $responseContent
                Headers = @{}
                StatusDescription = 'OK'
            }

            Write-C3DLog -Message "WebClient request successful" -Level Debug
            return $webResponse

        } catch [System.Net.WebException] {
            # Handle HTTP errors from WebClient
            $webException = $_.Exception
            $response = $webException.Response

            if ($response) {
                $statusCode = [int]$response.StatusCode
                $responseStream = $response.GetResponseStream()
                $reader = New-Object System.IO.StreamReader($responseStream)
                $responseContent = $reader.ReadToEnd()
                $reader.Close()
                $responseStream.Close()

                Write-C3DLog -Message "WebClient failed with HTTP $statusCode" -Level Error

                return [PSCustomObject]@{
                    StatusCode = $statusCode
                    Content = $responseContent
                    Headers = @{}
                    StatusDescription = $response.StatusDescription
                }
            } else {
                throw $webException
            }
        } finally {
            if ($webClient) {
                $webClient.Dispose()
            }
        }

    } else {
        # Use standard WebRequest for non-multipart requests
        Write-C3DLog -Message "Using System.Net.WebRequest for request" -Level Debug

        try {
            $request = [System.Net.WebRequest]::Create($Uri)
            $request.Method = $Method.ToString().ToUpper()
            $request.Headers.Add('Authorization', "APIKEY:DEVELOPER $ApiKey")
            $request.Headers.Add('User-Agent', 'C3DUploadTools-PowerShell/1.0')
            $request.Timeout = $TimeoutSeconds * 1000

            # Add additional headers
            foreach ($headerName in $Headers.Keys) {
                $request.Headers.Add($headerName, $Headers[$headerName])
            }

            # Add body for POST/PUT requests
            if ($Body) {
                $bodyBytes = if ($Body -is [byte[]]) {
                    $Body
                } elseif ($Body -is [string]) {
                    [System.Text.Encoding]::UTF8.GetBytes($Body)
                } else {
                    [System.Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 10))
                }

                $request.ContentLength = $bodyBytes.Length
                $request.ContentType = $ContentType

                Write-C3DLog -Message "Writing $($bodyBytes.Length) bytes to request stream" -Level Debug
                $requestStream = $request.GetRequestStream()
                $requestStream.Write($bodyBytes, 0, $bodyBytes.Length)
                $requestStream.Close()
            }

            # Get response
            Write-C3DLog -Message "Getting HTTP response..." -Level Debug
            $httpResponse = $request.GetResponse()
            $responseStream = $httpResponse.GetResponseStream()
            $reader = New-Object System.IO.StreamReader($responseStream)
            $responseContent = $reader.ReadToEnd()
            $reader.Close()
            $responseStream.Close()

            $webResponse = [PSCustomObject]@{
                StatusCode = [int]$httpResponse.StatusCode
                Content = $responseContent
                Headers = @{}
                StatusDescription = $httpResponse.StatusDescription
            }

            $httpResponse.Close()

            Write-C3DLog -Message "WebRequest completed successfully" -Level Debug
            return $webResponse

        } catch [System.Net.WebException] {
            $webException = $_.Exception
            $response = $webException.Response

            if ($response) {
                $statusCode = [int]$response.StatusCode
                $responseStream = $response.GetResponseStream()
                $reader = New-Object System.IO.StreamReader($responseStream)
                $responseContent = $reader.ReadToEnd()
                $reader.Close()
                $responseStream.Close()
                $response.Close()

                Write-C3DLog -Message "WebRequest failed with HTTP $statusCode" -Level Error

                return [PSCustomObject]@{
                    StatusCode = $statusCode
                    Content = $responseContent
                    Headers = @{}
                    StatusDescription = $response.StatusDescription
                }
            } else {
                throw $webException
            }
        }
    }
}

function ConvertTo-C3DApiResponse {
    <#
    .SYNOPSIS
        Converts raw HTTP response to standardized C3D API response object.

    .DESCRIPTION
        Processes HTTP response and creates a consistent response object with
        success indicators, timing information, and error details.

    .PARAMETER HttpResponse
        The raw HTTP response object.

    .PARAMETER TimingMs
        Request timing in milliseconds.

    .PARAMETER ErrorMessage
        Optional error message for failed requests.

    .EXAMPLE
        $apiResponse = ConvertTo-C3DApiResponse -HttpResponse $rawResponse -TimingMs 1500

    .OUTPUTS
        PSCustomObject with StatusCode, Body, Headers, TimingMs, Success, and Error properties
    #>

    [CmdletBinding()]
    [OutputType([PSCustomObject])]
    param(
        [Parameter(Mandatory)]
        [PSCustomObject]$HttpResponse,

        [Parameter(Mandatory)]
        [int]$TimingMs,

        [Parameter()]
        [string]$ErrorMessage
    )

    $isSuccess = $HttpResponse.StatusCode -ge 200 -and $HttpResponse.StatusCode -lt 300

    Write-C3DLog -Message "Converting HTTP response: $($HttpResponse.StatusCode) $($HttpResponse.StatusDescription)" -Level Debug
    Write-C3DLog -Message "Request timing: ${TimingMs}ms" -Level Debug

    # Provide specific guidance for common errors
    if (-not $isSuccess) {
        switch ($HttpResponse.StatusCode) {
            401 {
                Write-C3DLog -Message "Authentication failed (401 Unauthorized)" -Level Error
                Write-C3DLog -Message "Please check your C3D_DEVELOPER_API_KEY" -Level Error
                if ($HttpResponse.Content -like "*expired*") {
                    Write-C3DLog -Message "Your API key may have expired. Please generate a new one." -Level Error
                }
            }
            403 {
                Write-C3DLog -Message "Access forbidden (403 Forbidden)" -Level Error
                Write-C3DLog -Message "Your API key may not have permission for this operation" -Level Error
            }
            404 {
                Write-C3DLog -Message "Resource not found (404 Not Found)" -Level Error
                Write-C3DLog -Message "Please verify the scene ID or endpoint URL" -Level Error
            }
            429 {
                Write-C3DLog -Message "Rate limit exceeded (429 Too Many Requests)" -Level Error
                Write-C3DLog -Message "Please wait before retrying the request" -Level Error
            }
            500 {
                Write-C3DLog -Message "Server error (500 Internal Server Error)" -Level Error
                Write-C3DLog -Message "This may be a temporary issue. Please try again later." -Level Error
            }
            default {
                Write-C3DLog -Message "HTTP Error $($HttpResponse.StatusCode): $($HttpResponse.StatusDescription)" -Level Error
            }
        }

        if ($HttpResponse.Content) {
            Write-C3DLog -Message "Response body: $($HttpResponse.Content.Substring(0, [Math]::Min(1000, $HttpResponse.Content.Length)))" -Level Debug
        }
    }

    return [PSCustomObject]@{
        StatusCode = $HttpResponse.StatusCode
        Body = $HttpResponse.Content
        Headers = $HttpResponse.Headers
        TimingMs = $TimingMs
        Success = $isSuccess
        Error = if ($isSuccess) { $null } else { $ErrorMessage -or "HTTP $($HttpResponse.StatusCode): $($HttpResponse.StatusDescription)" }
        RawResponse = $HttpResponse
    }
}