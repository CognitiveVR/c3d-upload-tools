function Test-C3DApiKey {
    <#
    .SYNOPSIS
        Validates that the C3D API key is available and properly formatted.

    .DESCRIPTION
        Checks for C3D_DEVELOPER_API_KEY environment variable and validates its format.
        Equivalent to the bash validate_api_key() function in upload-utils.sh.
        Cross-platform compatible function.

    .PARAMETER Throw
        If specified, throws an exception when validation fails instead of returning false.

    .EXAMPLE
        if (-not (Test-C3DApiKey)) {
            Write-Error "API key validation failed"
            return
        }

    .EXAMPLE
        Test-C3DApiKey -Throw  # Will throw exception if invalid

    .OUTPUTS
        System.Boolean - True if API key is valid, false otherwise (unless -Throw is used)
    #>
    
    [CmdletBinding()]
    param(
        [switch]$Throw
    )
    
    $apiKey = $env:C3D_DEVELOPER_API_KEY
    
    if ([string]::IsNullOrEmpty($apiKey)) {
        $errorMessage = "C3D_DEVELOPER_API_KEY is not set. Please set it with: export C3D_DEVELOPER_API_KEY=your_api_key"
        
        Write-C3DLog -Message "C3D_DEVELOPER_API_KEY is not set" -Level Error
        
        # Provide cross-platform instructions
        if ($IsWindows -or $PSVersionTable.PSEdition -eq 'Desktop') {
            Write-C3DLog -Message "Set it with: `$env:C3D_DEVELOPER_API_KEY = 'your_api_key'" -Level Info
        } else {
            Write-C3DLog -Message "Set it with: export C3D_DEVELOPER_API_KEY='your_api_key'" -Level Info
        }
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # Basic API key format validation (should be a reasonable length)
    if ($apiKey.Length -lt 10) {
        $errorMessage = "C3D_DEVELOPER_API_KEY appears to be invalid (too short)"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # Check for obvious placeholder values
    $placeholderValues = @('your_api_key', 'test', 'example', 'placeholder', 'change_me')
    if ($apiKey -in $placeholderValues) {
        $errorMessage = "C3D_DEVELOPER_API_KEY appears to be a placeholder value: $apiKey"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    Write-C3DLog -Message "C3D_DEVELOPER_API_KEY has been set." -Level Info
    Write-C3DLog -Message "API key length: $($apiKey.Length) characters" -Level Debug
    
    return $true
}

function Get-C3DApiKey {
    <#
    .SYNOPSIS
        Retrieves the C3D API key with validation.

    .DESCRIPTION
        Gets the API key from environment variable with validation.
        Returns the key if valid, throws exception if invalid.

    .OUTPUTS
        System.String - The validated API key
    #>
    
    [CmdletBinding()]
    param()
    
    if (-not (Test-C3DApiKey -Throw)) {
        # This should not be reached due to -Throw, but included for safety
        throw "API key validation failed"
    }
    
    return $env:C3D_DEVELOPER_API_KEY
}