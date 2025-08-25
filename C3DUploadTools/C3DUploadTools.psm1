#Requires -Version 5.1

# C3DUploadTools PowerShell Module
# Main module file that loads and exports functions

# Set strict mode for better error handling
Set-StrictMode -Version 3.0

# Enable stopping on errors
$ErrorActionPreference = 'Stop'

# Get the module root path
$ModuleRoot = $PSScriptRoot

# Import private (helper) functions
Write-Verbose "Loading private functions..."
$PrivateFunctions = Get-ChildItem -Path "$ModuleRoot/Private" -Filter "*.ps1" -Recurse -ErrorAction SilentlyContinue
foreach ($Function in $PrivateFunctions) {
    try {
        Write-Verbose "Importing private function: $($Function.Name)"
        . $Function.FullName
    }
    catch {
        Write-Error "Failed to import private function $($Function.Name): $($_.Exception.Message)"
    }
}

# Import public (exported) functions
Write-Verbose "Loading public functions..."
$PublicFunctions = Get-ChildItem -Path "$ModuleRoot/Public" -Filter "*.ps1" -Recurse -ErrorAction SilentlyContinue
foreach ($Function in $PublicFunctions) {
    try {
        Write-Verbose "Importing public function: $($Function.Name)"
        . $Function.FullName
    }
    catch {
        Write-Error "Failed to import public function $($Function.Name): $($_.Exception.Message)"
    }
}

# Export public functions (alternative to manifest FunctionsToExport)
# This allows for dynamic function discovery
$FunctionNames = $PublicFunctions | ForEach-Object { [System.IO.Path]::GetFileNameWithoutExtension($_.Name) }
if ($FunctionNames) {
    Export-ModuleMember -Function $FunctionNames
    Write-Verbose "Exported functions: $($FunctionNames -join ', ')"
} else {
    Write-Warning "No public functions found to export"
}

# Module initialization
Write-Verbose "C3DUploadTools module loaded successfully"
Write-Verbose "Available functions: $($FunctionNames -join ', ')"

# Optional: Display module information when imported with -Verbose
# Note: $PSCmdlet is only available in advanced functions, not in module scope
# if ($VerbosePreference -eq 'Continue') {
#     $ModuleInfo = Get-Module -Name 'C3DUploadTools' -ListAvailable | Select-Object -First 1
#     if ($ModuleInfo) {
#         Write-Host "C3DUploadTools v$($ModuleInfo.Version) loaded" -ForegroundColor Green
#         Write-Host "Functions available: $($FunctionNames.Count)" -ForegroundColor Cyan
#     }
# }