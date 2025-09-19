function New-C3DMultipartFormData {
    <#
    .SYNOPSIS
        Creates multipart form data for file uploads.

    .DESCRIPTION
        Builds multipart form data body with proper boundaries and headers for uploading files to APIs.
        Handles multiple files and proper binary encoding.

    .PARAMETER FormData
        Hashtable of form field names to file paths.
        Example: @{ 'scene.bin' = '/path/to/scene.bin'; 'scene.gltf' = '/path/to/scene.gltf' }

    .PARAMETER Boundary
        The multipart boundary string to use. If not provided, a random boundary is generated.

    .EXAMPLE
        $formData = @{ 'file1' = 'C:\path\to\file.bin' }
        $result = New-C3DMultipartFormData -FormData $formData
        # Returns: @{ Body = [byte[]]; ContentType = 'multipart/form-data; boundary=...' }

    .OUTPUTS
        PSCustomObject with Body (byte array) and ContentType properties
    #>

    [CmdletBinding()]
    [OutputType([PSCustomObject])]
    param(
        [Parameter(Mandatory)]
        [hashtable]$FormData,

        [Parameter()]
        [string]$Boundary
    )

    # Generate boundary if not provided
    if (-not $Boundary) {
        $Boundary = "----C3DUploadBoundary$([System.Guid]::NewGuid().ToString('N'))"
    }

    Write-C3DLog -Message "Creating multipart form data with boundary: $Boundary" -Level Debug
    Write-C3DLog -Message "Files to include: $($FormData.Keys -join ', ')" -Level Debug

    # Validate all files exist first
    foreach ($fieldName in $FormData.Keys) {
        $filePath = $FormData[$fieldName]
        if (-not (Test-Path $filePath -PathType Leaf)) {
            throw "File not found for field '$fieldName': $filePath"
        }
    }

    # Calculate total size for logging
    $totalSize = 0
    foreach ($fieldName in $FormData.Keys) {
        $filePath = $FormData[$fieldName]
        $fileInfo = Get-Item $filePath
        $totalSize += $fileInfo.Length
        Write-C3DLog -Message "$fieldName`: $([math]::Round($fileInfo.Length / 1024, 2)) KB" -Level Debug
    }
    Write-C3DLog -Message "Total form data size: $([math]::Round($totalSize / 1KB, 2)) KB" -Level Info

    # Build multipart body
    $encoding = [System.Text.Encoding]::UTF8
    $newline = "`r`n"
    $bodyBytes = @()

    foreach ($fieldName in $FormData.Keys) {
        $filePath = $FormData[$fieldName]
        $fileName = [System.IO.Path]::GetFileName($filePath)

        Write-C3DLog -Message "Processing field: $fieldName -> $fileName" -Level Debug

        # Create field header
        $fieldHeader = "--$Boundary$newline"
        $fieldHeader += "Content-Disposition: form-data; name=`"$fieldName`"; filename=`"$fileName`"$newline"
        $fieldHeader += "Content-Type: application/octet-stream$newline$newline"

        # Convert header to bytes
        $headerBytes = $encoding.GetBytes($fieldHeader)

        # Read file content as bytes
        $fileBytes = [System.IO.File]::ReadAllBytes($filePath)

        # Add CRLF after file content
        $crlfBytes = $encoding.GetBytes($newline)

        # Combine header + file + CRLF
        $bodyBytes += $headerBytes
        $bodyBytes += $fileBytes
        $bodyBytes += $crlfBytes

        Write-C3DLog -Message "Added $($fileBytes.Length) bytes for $fieldName" -Level Debug
    }

    # Add closing boundary
    $closingBoundary = "--$Boundary--$newline"
    $bodyBytes += $encoding.GetBytes($closingBoundary)

    $contentType = "multipart/form-data; boundary=$Boundary"

    Write-C3DLog -Message "Multipart form data created: $($bodyBytes.Length) total bytes" -Level Info

    return [PSCustomObject]@{
        Body = $bodyBytes
        ContentType = $contentType
        Boundary = $Boundary
        TotalSize = $bodyBytes.Length
    }
}

function New-C3DSingleFileFormData {
    <#
    .SYNOPSIS
        Creates multipart form data for a single file upload.

    .DESCRIPTION
        Builds multipart form data for uploading a single file with the specified field name.

    .PARAMETER FilePath
        Path to the file to upload.

    .PARAMETER FieldName
        Form field name for the file upload.

    .PARAMETER Boundary
        The multipart boundary string to use. If not provided, a random boundary is generated.

    .EXAMPLE
        $result = New-C3DSingleFileFormData -FilePath 'C:\file.bin' -FieldName 'upload'

    .OUTPUTS
        PSCustomObject with Body (byte array) and ContentType properties
    #>

    [CmdletBinding()]
    [OutputType([PSCustomObject])]
    param(
        [Parameter(Mandatory)]
        [ValidateScript({Test-Path $_ -PathType Leaf})]
        [string]$FilePath,

        [Parameter(Mandatory)]
        [string]$FieldName,

        [Parameter()]
        [string]$Boundary
    )

    # Generate boundary if not provided
    if (-not $Boundary) {
        $Boundary = "----C3DUploadBoundary$([System.Guid]::NewGuid().ToString('N'))"
    }

    $fileInfo = Get-Item $FilePath
    $fileName = $fileInfo.Name
    $fileSizeKB = [math]::Round($fileInfo.Length / 1KB, 2)

    Write-C3DLog -Message "Creating single file form data for: $fileName ($fileSizeKB KB)" -Level Debug

    # Read file content
    $fileBytes = [System.IO.File]::ReadAllBytes($FilePath)

    # Build multipart body
    $encoding = [System.Text.Encoding]::UTF8
    $newline = "`r`n"

    # Create header
    $header = "--$Boundary$newline"
    $header += "Content-Disposition: form-data; name=`"$FieldName`"; filename=`"$fileName`"$newline"
    $header += "Content-Type: application/octet-stream$newline$newline"

    # Create footer
    $footer = "$newline--$Boundary--$newline"

    # Convert to bytes and combine
    $headerBytes = $encoding.GetBytes($header)
    $footerBytes = $encoding.GetBytes($footer)
    $bodyBytes = $headerBytes + $fileBytes + $footerBytes

    $contentType = "multipart/form-data; boundary=$Boundary"

    Write-C3DLog -Message "Single file form data created: $($bodyBytes.Length) total bytes" -Level Info

    return [PSCustomObject]@{
        Body = $bodyBytes
        ContentType = $contentType
        Boundary = $Boundary
        TotalSize = $bodyBytes.Length
    }
}