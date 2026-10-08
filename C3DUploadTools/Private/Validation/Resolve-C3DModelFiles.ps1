function Resolve-C3DModelFiles {
    <#
    .SYNOPSIS
        Picks the model files for a scene or dynamic object upload.

    .DESCRIPTION
        Equivalent to the bash resolve_model_files() function. A scene or object is
        uploaded as exactly one model: a single .glb, or the <BaseName>.gltf +
        <BaseName>.bin pair. Mixing the two forms, or holding more than one .glb,
        is an error — cvr-se-upload rejects the same shapes with a 400, so they are
        caught before anything is sent.

    .PARAMETER Directory
        Directory holding the upload's files.

    .PARAMETER BaseName
        Base name of the glTF Separate pair ('scene' for scenes; the object filename
        for objects). Without -AnyGlb it also names the .glb that is looked for.

    .PARAMETER AnyGlb
        Accept a single .glb of any name (scenes). Otherwise only <BaseName>.glb counts.

    .EXAMPLE
        $model = Resolve-C3DModelFiles -Directory $SceneDirectory -BaseName 'scene' -AnyGlb

    .EXAMPLE
        $model = Resolve-C3DModelFiles -Directory $ObjectDirectory -BaseName $ObjectFilename

    .OUTPUTS
        PSCustomObject with Format ('glb' or 'gltf') and Files (an ordered hashtable of
        upload file name -> full path, in upload order).
    #>

    [CmdletBinding()]
    [OutputType([PSCustomObject])]
    param(
        [Parameter(Mandatory)]
        [string]$Directory,

        [Parameter(Mandatory)]
        [string]$BaseName,

        [switch]$AnyGlb
    )

    $gltfPath = Join-Path -Path $Directory -ChildPath "$BaseName.gltf"
    $binPath = Join-Path -Path $Directory -ChildPath "$BaseName.bin"

    $glbFiles = @(Get-ChildItem -Path $Directory -File |
        Where-Object { $_.Extension -ieq '.glb' -and ($AnyGlb -or $_.BaseName -eq $BaseName) } |
        Sort-Object -Property Name)

    $separate = @()
    if (Test-Path -Path $gltfPath -PathType Leaf) { $separate += "$BaseName.gltf" }
    if (Test-Path -Path $binPath -PathType Leaf) { $separate += "$BaseName.bin" }

    if ($glbFiles.Count -gt 1) {
        throw "Found $($glbFiles.Count) .glb files in ${Directory} ($($glbFiles.Name -join ', ')). Keep exactly one .glb as the model."
    }
    if ($glbFiles.Count -eq 1 -and $separate.Count -gt 0) {
        throw "Found $($glbFiles[0].Name) alongside $($separate -join ', ') in ${Directory}. Upload either one .glb or $BaseName.gltf + $BaseName.bin, not both."
    }
    if ($glbFiles.Count -eq 1) {
        $files = [ordered]@{}
        $files[$glbFiles[0].Name] = $glbFiles[0].FullName
        Write-C3DLog -Message "Model (glb): $($glbFiles[0].Name)" -Level Debug
        return [PSCustomObject]@{ Format = 'glb'; Files = $files }
    }
    if ($separate.Count -eq 0) {
        $glbLabel = if ($AnyGlb) { 'one .glb' } else { "$BaseName.glb" }
        throw "No model found in ${Directory}: expected $glbLabel or $BaseName.gltf + $BaseName.bin"
    }
    if (-not (Test-Path -Path $gltfPath -PathType Leaf)) {
        throw "Required file missing: $gltfPath (needed with $BaseName.bin)"
    }
    if (-not (Test-Path -Path $binPath -PathType Leaf)) {
        throw "Required file missing: $binPath (needed with $BaseName.gltf)"
    }

    $files = [ordered]@{}
    $files["$BaseName.bin"] = $binPath
    $files["$BaseName.gltf"] = $gltfPath
    Write-C3DLog -Message "Model (gltf): $BaseName.bin, $BaseName.gltf" -Level Debug
    return [PSCustomObject]@{ Format = 'gltf'; Files = $files }
}
