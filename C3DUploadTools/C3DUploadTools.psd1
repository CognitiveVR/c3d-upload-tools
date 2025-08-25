@{
    # Basic module information
    RootModule = 'C3DUploadTools.psm1'
    ModuleVersion = '1.0.0'
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
    FunctionsToExport = @(
        'Upload-C3DScene',
        'Upload-C3DObject', 
        'Upload-C3DObjectManifest',
        'Get-C3DObjects',
        'Test-C3DUploads'
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
            ReleaseNotes = 'Initial release of PowerShell module for Cognitive3D upload tools'
            
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