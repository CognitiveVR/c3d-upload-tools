function Get-C3DGlbObjectBaseName {
    <#
    .SYNOPSIS
        The gateway's base name for a dynamic object's converted .bin and images.

    .DESCRIPTION
        Equivalent to the bash glb_object_base_name() function: the object id with
        every character outside [A-Za-z0-9_-] replaced by '_', at most 64
        characters (glbObjectBaseName in cvr-se-upload). Scenes use the base 'scene'.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]$ObjectId
    )

    $base = [regex]::Replace($ObjectId, '[^A-Za-z0-9_-]', '_')
    if ($base.Length -gt 64) { $base = $base.Substring(0, 64) }
    if ([string]::IsNullOrEmpty($base)) { return 'object' }
    return $base
}

function Test-C3DReservedConverterName {
    <#
    .SYNOPSIS
        Whether a file name is one the gateway gives an image extracted from a .glb.

    .DESCRIPTION
        Equivalent to the bash is_reserved_converter_name() function. True when
        FileName is <BaseName>_<n>.<ext>, matched in the base name's own case. The
        gateway refuses such a file beside a .glb (HTTP 400), because the
        converted output would collide with it.
    #>
    [CmdletBinding()]
    [OutputType([bool])]
    param(
        [Parameter(Mandatory)]
        [string]$FileName,

        [Parameter(Mandatory)]
        [string]$BaseName
    )

    $prefix = "$BaseName`_"
    if (-not $FileName.StartsWith($prefix, [System.StringComparison]::Ordinal)) { return $false }
    $rest = $FileName.Substring($prefix.Length)
    return [bool]($rest -cmatch '^[0-9]+\.[^.]+$')
}
