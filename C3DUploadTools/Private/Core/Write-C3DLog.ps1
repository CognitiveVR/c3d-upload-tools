function Write-C3DLog {
    <#
    .SYNOPSIS
        Centralized logging function for C3D upload tools.

    .DESCRIPTION
        Provides consistent logging with timestamps and color coding across all C3D functions.
        Matches the format and behavior of the bash upload-utils.sh logging functions.
        Supports different log levels and cross-platform color output.

    .PARAMETER Message
        The message to log.

    .PARAMETER Level
        Log level: Info, Warn, Error, Debug. Maps to bash log_info, log_warn, log_error, log_debug.

    .PARAMETER NoNewline
        Suppress newline after message.

    .EXAMPLE
        Write-C3DLog "Starting upload process" -Level Info
        Write-C3DLog "API key validation successful" -Level Info
        Write-C3DLog "Invalid file format detected" -Level Warn
        Write-C3DLog "Upload failed with HTTP 401" -Level Error
        Write-C3DLog "File size: 1.2MB" -Level Debug

    .NOTES
        - Info level uses Cyan color (matches bash COLOR_INFO)
        - Warn level uses Yellow color (matches bash COLOR_WARN)  
        - Error level uses Red color (matches bash COLOR_ERROR)
        - Debug level uses Gray color (matches bash COLOR_DEBUG)
        - Debug messages only display when $script:VerboseMode is enabled
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory, Position = 0)]
        [string]$Message,
        
        [ValidateSet('Info', 'Warn', 'Error', 'Debug')]
        [string]$Level = 'Info',
        
        [switch]$NoNewline
    )
    
    # Check if debug messages should be shown (equivalent to bash VERBOSE check)
    if ($Level -eq 'Debug' -and -not $script:VerboseMode) {
        return
    }
    
    # Format timestamp to match bash format: YYYY-MM-DD HH:MM:SS
    $timestamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
    $logMessage = "[$timestamp] [$($Level.ToUpper())] $Message"
    
    # Color mapping to match bash upload-utils.sh colors
    $colorMap = @{
        'Info'  = 'Cyan'    # COLOR_INFO="\033[1;34m" (bright blue, appears as cyan in most terminals)
        'Warn'  = 'Yellow'  # COLOR_WARN="\033[1;33m" 
        'Error' = 'Red'     # COLOR_ERROR="\033[1;31m"
        'Debug' = 'Gray'    # COLOR_DEBUG="\033[0;36m" (cyan, but we use gray for debug)
    }
    
    $writeParams = @{
        Object = $logMessage
        ForegroundColor = $colorMap[$Level]
    }
    
    if ($NoNewline) {
        $writeParams['NoNewline'] = $true
    }
    
    # Write the log message to console
    Write-Host @writeParams
}