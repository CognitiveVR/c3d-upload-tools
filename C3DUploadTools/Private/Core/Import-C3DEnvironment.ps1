function Import-C3DEnvironment {
    [CmdletBinding()]
    param(
        [string]$Path = '.env'
    )
    
    if (-not (Test-Path $Path)) {
        Write-C3DLog "No .env file found at: $Path" -Level Debug
        return
    }
    
    Write-C3DLog "Loading environment variables from: $Path" -Level Debug
    
    try {
        $envContent = Get-Content $Path -ErrorAction Stop
        $loadedCount = 0
        
        foreach ($line in $envContent) {
            # Skip empty lines and comments
            if ([string]::IsNullOrWhiteSpace($line) -or $line.TrimStart().StartsWith('#')) {
                continue
            }
            
            # Parse key=value pairs
            if ($line -match '^([^=]+)=(.*)$') {
                $key = $matches[1].Trim()
                $value = $matches[2].Trim()
                
                # Remove surrounding quotes if present
                if (($value.StartsWith('"') -and $value.EndsWith('"')) -or 
                    ($value.StartsWith("'") -and $value.EndsWith("'"))) {
                    $value = $value.Substring(1, $value.Length - 2)
                }
                
                # Only set if not already defined (preserves existing environment variables)
                if (-not (Get-Item "env:$key" -ErrorAction SilentlyContinue)) {
                    Set-Item "env:$key" $value
                    $loadedCount++
                    Write-C3DLog "Loaded environment variable: $key" -Level Debug
                }
                else {
                    Write-C3DLog "Environment variable '$key' already set, skipping" -Level Debug
                }
            }
        }
        
        if ($loadedCount -gt 0) {
            Write-C3DLog "Loaded $loadedCount environment variables from .env file" -Level Info
        }
        else {
            Write-C3DLog "No new environment variables loaded (all already set)" -Level Debug
        }
    }
    catch {
        Write-C3DLog "Failed to load .env file: $($_.Exception.Message)" -Level Warn
    }
}