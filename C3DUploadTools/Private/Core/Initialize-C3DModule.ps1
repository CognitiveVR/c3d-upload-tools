# Initialize-C3DModule.ps1
# Module initialization and script-level variables

# Script-level variables (equivalent to bash variables)
$script:VerboseMode = $false

# Helper functions to match bash logging function names
function script:log_info {
    param([string]$Message)
    Write-C3DLog -Message $Message -Level Info
}

function script:log_warn {
    param([string]$Message)  
    Write-C3DLog -Message $Message -Level Warn
}

function script:log_error {
    param([string]$Message)
    Write-C3DLog -Message $Message -Level Error
}

function script:log_debug {
    param([string]$Message)
    Write-C3DLog -Message $Message -Level Debug
}

# Backward compatibility alias for log_verbose
function script:log_verbose {
    param([string]$Message)
    Write-C3DLog -Message $Message -Level Debug
}

# Function to set verbose mode (equivalent to bash VERBOSE=true)
function Set-C3DVerboseMode {
    <#
    .SYNOPSIS
        Enable or disable verbose logging mode.
    
    .PARAMETER Enabled
        True to enable verbose/debug logging, false to disable.
    #>
    param([bool]$Enabled = $true)
    
    $script:VerboseMode = $Enabled
    if ($Enabled) {
        Write-C3DLog "Verbose mode enabled" -Level Debug
    }
}

# PowerShell equivalent of bash 'set -e' and 'set -u'
function Initialize-C3DStrictMode {
    <#
    .SYNOPSIS
        Initialize strict error handling mode equivalent to bash 'set -e' and 'set -u'.
    #>
    
    # Equivalent to 'set -e' - exit on errors
    $script:ErrorActionPreference = 'Stop'
    
    # Equivalent to 'set -u' - treat unset variables as errors  
    Set-StrictMode -Version 3.0
    
    Write-C3DLog "Strict mode initialized (ErrorActionPreference=Stop, StrictMode=3.0)" -Level Debug
}

# Note: Private functions are not exported by default
# For testing purposes, we may need to make some functions available