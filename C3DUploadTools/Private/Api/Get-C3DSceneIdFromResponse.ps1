function Get-C3DSceneIdFromResponse {
    <#
    .SYNOPSIS
        Reads the new scene ID from the body of a scene-create response.

    .DESCRIPTION
        The scene-create endpoint answers with the scene ID as plain text, sometimes
        wrapped in double quotes. Returns the ID in lowercase, or $null when the body
        is empty or is not a UUID.

    .PARAMETER Body
        The raw response body.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [AllowNull()]
        [AllowEmptyString()]
        [string]$Body
    )

    if ([string]::IsNullOrWhiteSpace($Body)) {
        return $null
    }

    $candidate = $Body.Trim().Trim('"').Trim()
    if ($candidate -match '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$') {
        return $candidate.ToLower()
    }
    return $null
}
