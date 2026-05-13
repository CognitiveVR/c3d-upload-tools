function Set-C3DRequestHeaders {
    <#
    .SYNOPSIS
        Applies a headers hashtable to a WebClient or HttpWebRequest, handling restricted headers correctly.

    .DESCRIPTION
        .NET treats User-Agent as a restricted header that cannot be set via Headers.Add() on any platform.
        WebClient requires the indexed HttpRequestHeader enum property; HttpWebRequest exposes a .UserAgent typed
        property — both throw ArgumentException if you call Headers.Add() for User-Agent.
        This function centralises that logic so callers don't need to handle it inline.

    .PARAMETER Request
        A System.Net.WebClient or System.Net.HttpWebRequest instance.

    .PARAMETER Headers
        Hashtable of header name/value pairs to apply.
    #>

    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [object]$Request,

        [Parameter(Mandatory)]
        [hashtable]$Headers
    )

    foreach ($headerName in $Headers.Keys) {
        if ($headerName -eq 'User-Agent') {
            if ($Request -is [System.Net.WebClient]) {
                $Request.Headers[[System.Net.HttpRequestHeader]::UserAgent] = $Headers[$headerName]
            } elseif ($Request -is [System.Net.HttpWebRequest]) {
                $Request.UserAgent = $Headers[$headerName]
            } else {
                throw [System.ArgumentException]::new(
                    "Set-C3DRequestHeaders: unsupported request type '$($Request.GetType().FullName)'. " +
                    "Expected System.Net.WebClient or System.Net.HttpWebRequest. " +
                    "Update this function to handle the new request type."
                )
            }
        } else {
            $Request.Headers.Add($headerName, $Headers[$headerName])
        }
    }
}
