@{
    # Basic module information
    RootModule = 'C3DUploadTools.psm1'
    # Keep ModuleVersion in lockstep with sdk-version.txt at the repo root.
    # Both bash and PowerShell upload paths read sdk-version.txt to build
    # the SDK telemetry prefix (cli-bash-v<version> / cli-powershell-v<version>).
    ModuleVersion = '1.1.0'
    GUID = 'f4e6d8c2-1a3b-4e5f-8c7d-2e9f1a6b3c4d'
    
    # Author and company information
    Author = 'Cognitive3D'
    CompanyName = 'Cognitive3D'
    Copyright = '(c) 2025 Cognitive3D. All rights reserved.'
    
    # Module description
    Description = 'PowerShell tools for uploading scenes and dynamic objects to the Cognitive3D analytics platform'
    
    # PowerShell version requirements - support both Windows PowerShell and PowerShell Core
    PowerShellVersion = '5.1'
    
    # Compatible PowerShell editions
    CompatiblePSEditions = @('Desktop', 'Core')
    
    # Functions to export - will be populated by the module
    # Test-C3DUploads is not exported: still a placeholder that throws
    # "Not implemented yet" (see Public/Test-C3DUploads.ps1). Re-add to
    # this list once a real implementation lands.
    FunctionsToExport = @(
        'Upload-C3DScene',
        'Upload-C3DObject',
        'Upload-C3DObjectManifest',
        'Get-C3DObjects'
    )
    
    # Cmdlets and variables to export (none for this module)
    CmdletsToExport = @()
    VariablesToExport = @()
    AliasesToExport = @()
    
    # Module metadata for PowerShell Gallery
    PrivateData = @{
        PSData = @{
            # Tags for PowerShell Gallery
            Tags = @('Cognitive3D', 'VR', 'Analytics', 'Upload', 'Scene', 'Objects', 'Cross-Platform')
            
            # License and project information
            LicenseUri = 'https://github.com/cognitive3d/c3d-upload-tools/blob/main/LICENSE'
            ProjectUri = 'https://github.com/cognitive3d/c3d-upload-tools'
            IconUri = 'https://cognitive3d.com/assets/images/cognitive3d-logo.png'
            
            # Release notes
            ReleaseNotes = @'
1.1.0
- Fix User-Agent restricted-header crash on Windows (Send-C3DHttpRequest)
- New Set-C3DRequestHeaders helper centralizes restricted-header handling
- Fix Get-C3DObjects and Upload-C3DObjectManifest reading the wrong response
  property (.Content vs .Body) - previously caused silent null/empty parses
- Capture StatusDescription before disposing the response object
  (HttpWebRequest error path)
- Fall back to .Error when response Body is null so transport errors render
  actionable messages instead of "HTTP : "
- Fix .Error population using boolean -or instead of a real string fallback
- Drop the Test-C3DUploads placeholder from public exports until it has a
  real implementation
- Add LICENSE file (Cognitive3D SDK Software License)
- Correct LicenseUri and ProjectUri to the actual GitHub org slug

1.0.0
- Initial PowerShell module for Cognitive3D upload tools
'@
            
            # Prerelease information (remove for stable release)
            # Prerelease = 'beta'
            
            # External module dependencies
            # ExternalModuleDependencies = @()
        }
    }
    
    # Help information
    HelpInfoURI = 'https://docs.cognitive3d.com/upload-tools/powershell'
    
    # Default prefix for imported commands (optional)
    # DefaultCommandPrefix = 'C3D'
}