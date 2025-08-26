function Invoke-C3DApiRequest {
    <#
    .SYNOPSIS
        Makes HTTP requests to the Cognitive3D API with enhanced error handling.

    .DESCRIPTION
        PowerShell equivalent of the curl commands used in bash scripts.
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
        
        # Prepare headers - avoid PowerShell's Authorization header validation
        $requestHeaders = $Headers.Clone()
        $requestHeaders['User-Agent'] = 'C3DUploadTools-PowerShell/1.0'
        
        Write-C3DLog -Message "Request headers prepared (Authorization: [REDACTED])" -Level Debug
        
        # Prepare request parameters without Authorization in headers
        # We'll add it directly to the WebRequest object to bypass validation
        $requestParams = @{
            Uri = $Uri
            Method = $Method
            Headers = $requestHeaders
            TimeoutSec = $TimeoutSeconds
            UseBasicParsing = $true
        }
        
        # Handle different request types
        if ($FormData) {
            Write-C3DLog -Message "Preparing multipart form data upload with $($FormData.Count) files" -Level Info
            
            # Create multipart form data for multiple files
            $boundary = "----C3DUploadBoundary$([System.Guid]::NewGuid().ToString('N'))"
            $requestParams['ContentType'] = "multipart/form-data; boundary=$boundary"
            
            # Calculate total size for progress
            $totalSize = 0
            foreach ($fieldName in $FormData.Keys) {
                $filePath = $FormData[$fieldName]
                if (-not (Test-Path $filePath -PathType Leaf)) {
                    throw "File not found for field '$fieldName': $filePath"
                }
                $fileInfo = Get-Item $filePath
                $totalSize += $fileInfo.Length
                Write-C3DLog -Message "$fieldName`: $([math]::Round($fileInfo.Length / 1024, 2)) KB" -Level Debug
            }
            Write-C3DLog -Message "Total upload size: $([math]::Round($totalSize / 1024, 2)) KB" -Level Info
            
            # Build multipart body with proper binary handling
            $bodyBytes = @()
            
            foreach ($fieldName in $FormData.Keys) {
                $filePath = $FormData[$fieldName]
                $fileInfo = Get-Item $filePath
                
                # Add boundary and headers as text
                $headerText = "--$boundary`r`n"
                $headerText += "Content-Disposition: form-data; name=`"$fieldName`"; filename=`"$($fileInfo.Name)`"`r`n"
                $headerText += "Content-Type: application/octet-stream`r`n`r`n"
                $headerBytes = [System.Text.Encoding]::UTF8.GetBytes($headerText)
                
                # Read file content as bytes
                $fileBytes = [System.IO.File]::ReadAllBytes($filePath)
                
                # Add CRLF after file content
                $crlfBytes = [System.Text.Encoding]::UTF8.GetBytes("`r`n")
                
                # Combine header + file + CRLF
                $bodyBytes += $headerBytes
                $bodyBytes += $fileBytes
                $bodyBytes += $crlfBytes
            }
            
            # Add final boundary
            $finalBoundary = "--$boundary--`r`n"
            $finalBoundaryBytes = [System.Text.Encoding]::UTF8.GetBytes($finalBoundary)
            $bodyBytes += $finalBoundaryBytes
            
            $requestParams['Body'] = $bodyBytes
            
        } elseif ($FilePath) {
            Write-C3DLog -Message "Preparing multipart file upload: $FilePath" -Level Info
            
            # Get file information
            $fileInfo = Get-Item $FilePath
            $fileSizeKB = [math]::Round($fileInfo.Length / 1024, 2)
            Write-C3DLog -Message "File size: $fileSizeKB KB" -Level Debug
            
            # Create multipart form data
            $boundary = "----C3DUploadBoundary$([System.Guid]::NewGuid().ToString('N'))"
            $requestParams['ContentType'] = "multipart/form-data; boundary=$boundary"
            
            # Read file content
            $fileBytes = [System.IO.File]::ReadAllBytes($FilePath)
            
            # Build multipart body
            $bodyLines = @()
            $bodyLines += "--$boundary"
            $bodyLines += "Content-Disposition: form-data; name=`"$FieldName`"; filename=`"$($fileInfo.Name)`""
            $bodyLines += "Content-Type: application/octet-stream"
            $bodyLines += ""
            
            # Convert to bytes and combine with file content
            $bodyText = ($bodyLines -join "`r`n") + "`r`n"
            $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyText)
            $endBytes = [System.Text.Encoding]::UTF8.GetBytes("`r`n--$boundary--`r`n")
            
            $requestParams['Body'] = $bodyBytes + $fileBytes + $endBytes
            
        } elseif ($Body) {
            Write-C3DLog -Message "Preparing JSON request body" -Level Debug
            
            if ($Body -is [string]) {
                $requestParams['Body'] = $Body
            } else {
                $requestParams['Body'] = $Body | ConvertTo-Json -Depth 10
            }
            $requestParams['ContentType'] = $ContentType
        }
        
        # Show progress for large requests
        if ($FormData -or $FilePath -or ($Body -and $Body.Length -gt 10000)) {
            Write-Progress -Activity $ProgressTitle -Status "Sending request..." -PercentComplete 25
        }
        
        Write-C3DLog -Message "Invoking web request..." -Level Debug
        
        # PowerShell Invoke-WebRequest validates Authorization headers strictly
        # Use System.Net.WebClient for multipart uploads to bypass validation
        if ($FormData -and $FormData.Count -gt 0) {
            Write-C3DLog -Message "Using System.Net.WebClient for multipart upload (Windows compatible)" -Level Info
            
            try {
                # Use WebClient which allows custom headers without validation
                $webClient = New-Object System.Net.WebClient
                $webClient.Headers.Add('Authorization', "APIKEY:DEVELOPER $ApiKey")
                $webClient.Headers.Add('User-Agent', 'C3DUploadTools-PowerShell/1.0')
                
                # Create boundary for multipart data
                $boundary = "----C3DUploadBoundary$([System.Guid]::NewGuid().ToString('N'))"
                $webClient.Headers.Add('Content-Type', "multipart/form-data; boundary=$boundary")
                
                # Build multipart form data manually
                $encoding = [System.Text.Encoding]::UTF8
                $newline = "`r`n"
                
                $formDataBytes = @()
                
                foreach ($fieldName in $FormData.Keys) {
                    $filePath = $FormData[$fieldName]
                    $fileName = [System.IO.Path]::GetFileName($filePath)
                    $fileBytes = [System.IO.File]::ReadAllBytes($filePath)
                    
                    # Add field header
                    $fieldHeader = "--$boundary$newline"
                    $fieldHeader += "Content-Disposition: form-data; name=`"$fieldName`"; filename=`"$fileName`"$newline"
                    $fieldHeader += "Content-Type: application/octet-stream$newline$newline"
                    
                    $headerBytes = $encoding.GetBytes($fieldHeader)
                    $endBytes = $encoding.GetBytes($newline)
                    
                    $formDataBytes += $headerBytes
                    $formDataBytes += $fileBytes
                    $formDataBytes += $endBytes
                }
                
                # Add closing boundary
                $closingBoundary = "--$boundary--$newline"
                $formDataBytes += $encoding.GetBytes($closingBoundary)
                
                # Upload data
                Write-C3DLog -Message "Uploading $($formDataBytes.Length) bytes via WebClient" -Level Debug
                $responseBytes = $webClient.UploadData($Uri, 'POST', $formDataBytes)
                $responseContent = $encoding.GetString($responseBytes)
                
                # WebClient doesn't provide status code directly for successful uploads
                # If we get here without exception, assume success (200)
                $webResponse = [PSCustomObject]@{
                    StatusCode = 200
                    Content = $responseContent
                    Headers = @{}
                    StatusDescription = 'OK'
                }
                
                Write-C3DLog -Message "WebClient upload successful" -Level Debug
                $webClient.Dispose()
                
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
                    Write-C3DLog -Message "Response: $responseContent" -Level Debug
                    
                    $webResponse = [PSCustomObject]@{
                        StatusCode = $statusCode
                        Content = $responseContent
                        Headers = @{}
                        StatusDescription = $response.StatusDescription
                    }
                } else {
                    throw $webException
                }
                
                if ($webClient) { $webClient.Dispose() }
            }
            
        } else {
            # For non-multipart requests, use standard PowerShell with custom auth handling
            # Create a custom WebRequest to bypass Authorization header validation
            try {
                $request = [System.Net.WebRequest]::Create($Uri)
                $request.Method = $Method
                $request.Headers.Add('Authorization', "APIKEY:DEVELOPER $ApiKey")
                $request.Headers.Add('User-Agent', 'C3DUploadTools-PowerShell/1.0')
                $request.Timeout = $TimeoutSeconds * 1000
                
                # Add body for POST/PUT requests
                if ($Body) {
                    $bodyBytes = if ($Body -is [string]) { 
                        [System.Text.Encoding]::UTF8.GetBytes($Body) 
                    } else { 
                        [System.Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 10))
                    }
                    
                    $request.ContentLength = $bodyBytes.Length
                    $request.ContentType = $ContentType
                    
                    $requestStream = $request.GetRequestStream()
                    $requestStream.Write($bodyBytes, 0, $bodyBytes.Length)
                    $requestStream.Close()
                }
                
                # Get response
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
                    
                    $webResponse = [PSCustomObject]@{
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
        
        # Calculate timing
        $endTime = Get-Date
        $timingMs = [math]::Round(($endTime - $startTime).TotalMilliseconds, 0)
        
        Write-C3DLog -Message "Request completed in ${timingMs}ms" -Level Info
        Write-C3DLog -Message "Response status: $($webResponse.StatusCode) $($webResponse.StatusDescription)" -Level Info
        
        # Clear progress
        if ($FormData -or $FilePath -or ($Body -and $Body.Length -gt 10000)) {
            Write-Progress -Activity $ProgressTitle -Completed
        }
        
        # Return structured response
        return [PSCustomObject]@{
            StatusCode = $webResponse.StatusCode
            Body = $webResponse.Content
            Headers = $webResponse.Headers
            TimingMs = $timingMs
            Success = $true
            RawResponse = $webResponse
        }
        
    } catch [Microsoft.PowerShell.Commands.HttpResponseException] {
        $endTime = Get-Date
        $timingMs = [math]::Round(($endTime - $startTime).TotalMilliseconds, 0)
        
        Write-Progress -Activity $ProgressTitle -Completed
        
        $statusCode = [int]$_.Exception.Response.StatusCode
        $responseBody = $_.Exception.Response.Content.ReadAsStringAsync().Result
        
        Write-C3DLog -Message "HTTP request failed after ${timingMs}ms" -Level Error
        Write-C3DLog -Message "Status Code: $statusCode" -Level Error
        Write-C3DLog -Message "Response body: $($responseBody.Substring(0, [Math]::Min(1000, $responseBody.Length)))" -Level Error
        
        # Return error response object
        return [PSCustomObject]@{
            StatusCode = $statusCode
            Body = $responseBody
            Headers = $null
            TimingMs = $timingMs
            Success = $false
            Error = $_.Exception.Message
            RawResponse = $null
        }
        
    } catch [System.Net.WebException] {
        $endTime = Get-Date
        $timingMs = [math]::Round(($endTime - $startTime).TotalMilliseconds, 0)
        
        Write-Progress -Activity $ProgressTitle -Completed
        
        $statusCode = $null
        $responseBody = $null
        
        if ($_.Exception.Response) {
            $statusCode = [int]$_.Exception.Response.StatusCode
            
            try {
                $responseStream = $_.Exception.Response.GetResponseStream()
                $reader = New-Object System.IO.StreamReader($responseStream)
                $responseBody = $reader.ReadToEnd()
                $reader.Close()
                $responseStream.Close()
            } catch {
                $responseBody = "Unable to read response body"
            }
        }
        
        Write-C3DLog -Message "HTTP request failed after ${timingMs}ms" -Level Error
        Write-C3DLog -Message "Status Code: $statusCode" -Level Error
        
        # Provide specific guidance for common errors
        switch ($statusCode) {
            401 {
                Write-C3DLog -Message "Authentication failed (401 Unauthorized)" -Level Error
                Write-C3DLog -Message "Please check your C3D_DEVELOPER_API_KEY" -Level Error
                if ($responseBody -like "*expired*") {
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
                Write-C3DLog -Message "HTTP Error: $($_.Exception.Message)" -Level Error
            }
        }
        
        if ($responseBody) {
            Write-C3DLog -Message "Response body: $responseBody" -Level Debug
        }
        
        # Return error response object
        return [PSCustomObject]@{
            StatusCode = $statusCode
            Body = $responseBody
            Headers = $null
            TimingMs = $timingMs
            Success = $false
            Error = $_.Exception.Message
            RawResponse = $null
        }
        
    } catch {
        $endTime = Get-Date
        $timingMs = [math]::Round(($endTime - $startTime).TotalMilliseconds, 0)
        
        Write-Progress -Activity $ProgressTitle -Completed
        
        Write-C3DLog -Message "Request failed after ${timingMs}ms: $($_.Exception.Message)" -Level Error
        
        # Return error response object
        return [PSCustomObject]@{
            StatusCode = $null
            Body = $null
            Headers = $null
            TimingMs = $timingMs
            Success = $false
            Error = $_.Exception.Message
            RawResponse = $null
        }
    }
}