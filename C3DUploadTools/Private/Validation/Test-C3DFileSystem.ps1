function Test-C3DDirectory {
    <#
    .SYNOPSIS
        Validates that a directory exists and is accessible.

    .DESCRIPTION
        Equivalent to the bash validate_directory() function.
        Checks that the specified path exists, is a directory, and is accessible.

    .PARAMETER Path
        The directory path to validate.

    .PARAMETER Name
        Descriptive name for the directory (for error messages).

    .PARAMETER RequiredFiles
        Optional array of filenames that must exist in the directory.

    .PARAMETER Throw
        If specified, throws an exception when validation fails.

    .EXAMPLE
        Test-C3DDirectory -Path "C:\Scenes\MyScene" -Name "scene directory"

    .EXAMPLE
        $requiredFiles = @('scene.bin', 'scene.gltf', 'screenshot.png', 'settings.json')
        Test-C3DDirectory -Path $scenePath -Name "scene directory" -RequiredFiles $requiredFiles -Throw

    .OUTPUTS
        System.Boolean - True if directory is valid, false otherwise (unless -Throw is used)
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        
        [Parameter()]
        [string]$Name = "directory",
        
        [Parameter()]
        [string[]]$RequiredFiles = @(),
        
        [switch]$Throw
    )
    
    Write-C3DLog -Message "Validating $Name`: $Path" -Level Debug
    
    # Check if path exists
    if (-not (Test-Path -Path $Path)) {
        $errorMessage = "$Name does not exist: $Path"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # Check if it's a directory
    if (-not (Test-Path -Path $Path -PathType Container)) {
        $errorMessage = "$Name is not a directory: $Path"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # Get directory info for additional checks
    try {
        $dirInfo = Get-Item -Path $Path -ErrorAction Stop
        Write-C3DLog -Message "$Name exists: $($dirInfo.FullName)" -Level Debug
    } catch {
        $errorMessage = "Cannot access $Name`: $($_.Exception.Message)"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # Check for required files
    if ($RequiredFiles.Count -gt 0) {
        $missingFiles = @()
        
        foreach ($fileName in $RequiredFiles) {
            $filePath = Join-Path -Path $Path -ChildPath $fileName
            if (-not (Test-Path -Path $filePath -PathType Leaf)) {
                $missingFiles += $fileName
            } else {
                Write-C3DLog -Message "Found required file: $fileName" -Level Debug
            }
        }
        
        if ($missingFiles.Count -gt 0) {
            $errorMessage = "Missing required files in $Name`: $($missingFiles -join ', ')"
            Write-C3DLog -Message $errorMessage -Level Error
            Write-C3DLog -Message "Required files: $($RequiredFiles -join ', ')" -Level Error
            
            if ($Throw) {
                throw $errorMessage
            }
            return $false
        }
        
        Write-C3DLog -Message "All required files found in $Name" -Level Debug
    }
    
    Write-C3DLog -Message "$Name validation successful" -Level Debug
    return $true
}

function Test-C3DFile {
    <#
    .SYNOPSIS
        Validates that a file exists and meets size requirements.

    .DESCRIPTION
        Comprehensive file validation with size limits and format checking.

    .PARAMETER Path
        The file path to validate.

    .PARAMETER Name
        Descriptive name for the file (for error messages).

    .PARAMETER MaxSizeBytes
        Maximum file size in bytes. Defaults to 100MB.

    .PARAMETER RequiredExtensions
        Array of allowed file extensions (with or without dots).

    .PARAMETER Throw
        If specified, throws an exception when validation fails.

    .EXAMPLE
        Test-C3DFile -Path "scene.bin" -Name "scene binary file" -MaxSizeBytes 50MB

    .EXAMPLE
        Test-C3DFile -Path "model.gltf" -RequiredExtensions @('.gltf', '.glb') -Throw

    .OUTPUTS
        System.Boolean - True if file is valid, false otherwise (unless -Throw is used)
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        
        [Parameter()]
        [string]$Name = "file",
        
        [Parameter()]
        [long]$MaxSizeBytes = 100MB,
        
        [Parameter()]
        [string[]]$RequiredExtensions = @(),
        
        [switch]$Throw
    )
    
    Write-C3DLog -Message "Validating $Name`: $Path" -Level Debug
    
    # Check if file exists
    if (-not (Test-Path -Path $Path -PathType Leaf)) {
        $errorMessage = "$Name does not exist: $Path"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # Get file info
    try {
        $fileInfo = Get-Item -Path $Path -ErrorAction Stop
        Write-C3DLog -Message "$Name exists: $($fileInfo.FullName)" -Level Debug
    } catch {
        $errorMessage = "Cannot access $Name`: $($_.Exception.Message)"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # Check file size
    $fileSizeKB = [math]::Round($fileInfo.Length / 1024, 2)
    $fileSizeMB = [math]::Round($fileInfo.Length / 1MB, 2)
    
    Write-C3DLog -Message "$Name size: $fileSizeKB KB ($fileSizeMB MB)" -Level Debug
    
    if ($fileInfo.Length -gt $MaxSizeBytes) {
        $maxSizeMB = [math]::Round($MaxSizeBytes / 1MB, 2)
        $errorMessage = "$Name is too large: $fileSizeMB MB (maximum: $maxSizeMB MB)"
        Write-C3DLog -Message $errorMessage -Level Error
        
        if ($Throw) {
            throw $errorMessage
        }
        return $false
    }
    
    # Check file extension
    if ($RequiredExtensions.Count -gt 0) {
        $fileExtension = $fileInfo.Extension
        
        # Normalize extensions (ensure they start with dot)
        $normalizedExtensions = $RequiredExtensions | ForEach-Object {
            if ($_.StartsWith('.')) { $_ } else { ".$_" }
        }
        
        if ($fileExtension -notin $normalizedExtensions) {
            $errorMessage = "$Name has invalid extension: $fileExtension (allowed: $($normalizedExtensions -join ', '))"
            Write-C3DLog -Message $errorMessage -Level Error
            
            if ($Throw) {
                throw $errorMessage
            }
            return $false
        }
        
        Write-C3DLog -Message "$Name extension is valid: $fileExtension" -Level Debug
    }
    
    Write-C3DLog -Message "$Name validation successful" -Level Debug
    return $true
}

function Get-C3DFileSize {
    <#
    .SYNOPSIS
        Gets file size information in a formatted way.

    .DESCRIPTION
        Returns file size in bytes, KB, and MB with human-readable formatting.

    .PARAMETER Path
        Path to the file.

    .EXAMPLE
        Get-C3DFileSize -Path "scene.bin"
        Returns: PSObject with Bytes, KB, MB, and FormattedSize properties

    .OUTPUTS
        PSCustomObject with size information
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [ValidateScript({Test-Path $_ -PathType Leaf})]
        [string]$Path
    )
    
    $fileInfo = Get-Item -Path $Path
    $bytes = $fileInfo.Length
    $kb = [math]::Round($bytes / 1KB, 2)
    $mb = [math]::Round($bytes / 1MB, 2)
    
    # Format size appropriately
    if ($mb -ge 1) {
        $formatted = "$mb MB"
    } elseif ($kb -ge 1) {
        $formatted = "$kb KB"
    } else {
        $formatted = "$bytes bytes"
    }
    
    return [PSCustomObject]@{
        Bytes = $bytes
        KB = $kb
        MB = $mb
        FormattedSize = $formatted
        Path = $fileInfo.FullName
        Name = $fileInfo.Name
    }
}

function Backup-C3DFile {
    <#
    .SYNOPSIS
        Creates a backup copy of a file with timestamp.

    .DESCRIPTION
        Creates a backup file with .bak extension and timestamp for safe file operations.
        Returns the backup file path for rollback operations.

    .PARAMETER Path
        Path to the file to backup.

    .PARAMETER BackupDirectory
        Optional directory for backup files. Defaults to same directory as original.

    .EXAMPLE
        $backupPath = Backup-C3DFile -Path "settings.json"
        # Creates settings.json.20250825-143022.bak

    .OUTPUTS
        System.String - Path to the backup file
    #>
    
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [ValidateScript({Test-Path $_ -PathType Leaf})]
        [string]$Path,
        
        [Parameter()]
        [string]$BackupDirectory
    )
    
    $fileInfo = Get-Item -Path $Path
    $timestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    
    if ($BackupDirectory) {
        $backupPath = Join-Path -Path $BackupDirectory -ChildPath "$($fileInfo.Name).$timestamp.bak"
    } else {
        $backupPath = "$($fileInfo.FullName).$timestamp.bak"
    }
    
    try {
        Copy-Item -Path $Path -Destination $backupPath -Force
        Write-C3DLog -Message "Created backup: $backupPath" -Level Debug
        return $backupPath
    } catch {
        Write-C3DLog -Message "Failed to create backup: $($_.Exception.Message)" -Level Error
        throw "Backup operation failed: $($_.Exception.Message)"
    }
}