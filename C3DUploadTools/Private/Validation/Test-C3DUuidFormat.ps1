function Test-C3DUuidFormat {
    <#
    .SYNOPSIS
        Validates that a string matches the UUID format.

    .DESCRIPTION
        Equivalent to the bash validate_uuid_format() function in upload-utils.sh.
        Validates that the input string matches the standard UUID format:
        xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (8-4-4-4-12 hexadecimal characters)

    .PARAMETER Uuid
        The UUID string to validate.

    .PARAMETER FieldName
        Optional name of the field being validated for better error messages.
        Defaults to "UUID".

    .PARAMETER Throw
        If specified, throws an exception when validation fails instead of returning false.

    .EXAMPLE
        Test-C3DUuidFormat -Uuid "12345678-1234-1234-1234-123456789012"
        Returns: $true

    .EXAMPLE
        Test-C3DUuidFormat -Uuid "invalid-uuid" -FieldName "scene_id" -Throw
        Throws exception with descriptive error message

    .OUTPUTS
        System.Boolean - True if UUID format is valid, false otherwise (unless -Throw is used)

    .NOTES
        The UUID format validated is: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
        where x represents a hexadecimal digit (0-9, a-f, A-F)
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory, Position = 0)]
        [AllowEmptyString()]
        [string]$Uuid,
        
        [Parameter()]
        [string]$FieldName = "UUID",
        
        [switch]$Throw
    )
    
    Write-C3DLog -Message "Validating UUID format for $FieldName`: $Uuid" -Level Debug
    
    # Check if empty
    if ([string]::IsNullOrEmpty($Uuid)) {
        $errorMessage = "$FieldName is empty"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # UUID regex pattern: 8-4-4-4-12 hexadecimal characters
    # Matches both uppercase and lowercase hex digits
    $uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
    
    if ($Uuid -notmatch $uuidPattern) {
        $errorMessage = "Invalid $FieldName format: $Uuid"
        Write-C3DLog -Message $errorMessage -Level Error
        Write-C3DLog -Message "Expected UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    Write-C3DLog -Message "$FieldName format is valid" -Level Debug
    return $true
}

function ConvertTo-C3DLowerUuid {
    <#
    .SYNOPSIS
        Converts a UUID to lowercase format.

    .DESCRIPTION
        Ensures UUID is in lowercase format for consistency with API requirements.
        Also validates the UUID format in the process.

    .PARAMETER Uuid
        The UUID string to convert.

    .PARAMETER FieldName
        Optional name of the field for better error messages.

    .EXAMPLE
        ConvertTo-C3DLowerUuid -Uuid "12345678-1234-1234-1234-123456789012"
        Returns: "12345678-1234-1234-1234-123456789012"

    .EXAMPLE
        ConvertTo-C3DLowerUuid -Uuid "ABCDEF01-2345-6789-ABCD-EF0123456789"
        Returns: "abcdef01-2345-6789-abcd-ef0123456789"

    .OUTPUTS
        System.String - The UUID in lowercase format
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory, Position = 0)]
        [string]$Uuid,
        
        [Parameter()]
        [string]$FieldName = "UUID"
    )
    
    # Validate format first
    if (-not (Test-C3DUuidFormat -Uuid $Uuid -FieldName $FieldName -Throw)) {
        # This should not be reached due to -Throw, but included for safety
        throw "UUID validation failed"
    }
    
    $lowerUuid = $Uuid.ToLowerInvariant()
    
    if ($Uuid -cne $lowerUuid) {
        Write-C3DLog -Message "Converted $FieldName to lowercase: $Uuid -> $lowerUuid" -Level Debug
    }
    
    return $lowerUuid
}

function New-C3DUuid {
    <#
    .SYNOPSIS
        Generates a new UUID in the correct format.

    .DESCRIPTION
        Creates a new UUID using .NET's Guid class and formats it consistently
        with the C3D API requirements (lowercase).

    .EXAMPLE
        New-C3DUuid
        Returns: A new UUID like "12345678-1234-1234-1234-123456789012"

    .OUTPUTS
        System.String - A new UUID in lowercase format
    #>
    
    [CmdletBinding()]
    param()
    
    $newUuid = [System.Guid]::NewGuid().ToString().ToLowerInvariant()
    Write-C3DLog -Message "Generated new UUID: $newUuid" -Level Debug
    
    return $newUuid
}