function Get-C3DApiUrl {
    <#
    .SYNOPSIS
        Generates the appropriate API base URL based on environment and endpoint type.

    .DESCRIPTION
        Equivalent to the bash get_api_base_url() function in upload-utils.sh.
        Returns the correct API endpoint URL for the specified environment and endpoint type.

    .PARAMETER Environment
        The target environment: 'prod' for production or 'dev' for development.

    .PARAMETER EndpointType
        The type of endpoint: 'scenes', 'objects', or other API endpoint types.
        Defaults to 'scenes' for backward compatibility.

    .EXAMPLE
        Get-C3DApiUrl -Environment 'prod' -EndpointType 'scenes'
        Returns: "https://data.cognitive3d.com/v0/scenes"

    .EXAMPLE
        Get-C3DApiUrl -Environment 'dev' -EndpointType 'objects'
        Returns: "https://data.c3ddev.com/v0/objects"

    .OUTPUTS
        System.String - The complete API endpoint URL

    .NOTES
        Supported environments:
        - prod: https://data.cognitive3d.com/v0/
        - dev: https://data.c3ddev.com/v0/
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [ValidateSet('prod', 'dev')]
        [string]$Environment,
        
        [Parameter()]
        [ValidateNotNullOrEmpty()]
        [string]$EndpointType = 'scenes'
    )
    
    Write-C3DLog -Message "Getting API URL for environment: $Environment, endpoint: $EndpointType" -Level Debug
    
    # Base URLs for each environment (matching bash script)
    $baseUrls = @{
        'prod' = 'https://data.cognitive3d.com/v0'
        'dev'  = 'https://data.c3ddev.com/v0'
    }
    
    if (-not $baseUrls.ContainsKey($Environment)) {
        throw "Unknown environment: $Environment. Valid environments are: $($baseUrls.Keys -join ', ')"
    }
    
    $baseUrl = $baseUrls[$Environment]
    $fullUrl = "$baseUrl/$EndpointType"
    
    Write-C3DLog -Message "Generated API URL: $fullUrl" -Level Debug
    
    return $fullUrl
}

function Test-C3DEnvironment {
    <#
    .SYNOPSIS
        Validates that the specified environment is supported.

    .DESCRIPTION
        Equivalent to the bash validate_environment() function in upload-utils.sh.
        Validates that the environment parameter is either 'prod' or 'dev'.

    .PARAMETER Environment
        The environment to validate.

    .PARAMETER Throw
        If specified, throws an exception when validation fails instead of returning false.

    .EXAMPLE
        if (-not (Test-C3DEnvironment -Environment $userInput)) {
            Write-Error "Invalid environment"
            return
        }

    .OUTPUTS
        System.Boolean - True if environment is valid, false otherwise (unless -Throw is used)
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Environment,
        
        [switch]$Throw
    )
    
    $validEnvironments = @('prod', 'dev')
    
    if ($Environment -notin $validEnvironments) {
        $errorMessage = "Invalid environment: $Environment. Must be 'prod' or 'dev'."
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    Write-C3DLog -Message "Environment validated: $Environment" -Level Debug
    return $true
}