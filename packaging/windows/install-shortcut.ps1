param(
    [Parameter(Mandatory = $true)]
    [string]$TargetPath,

    [Parameter(Mandatory = $true)]
    [string]$IconPath,

    [string]$ProgramsDirectory = ""
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($ProgramsDirectory)) {
    $ProgramsDirectory = [Environment]::GetFolderPath("Programs")
}
if ([string]::IsNullOrWhiteSpace($ProgramsDirectory)) {
    throw "Windows Start Menu Programs directory was not found."
}

[System.IO.Directory]::CreateDirectory($ProgramsDirectory) | Out-Null
$shortcutPath = Join-Path $ProgramsDirectory "Growse.lnk"
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $TargetPath
$shortcut.WorkingDirectory = Split-Path -Parent $TargetPath
$shortcut.IconLocation = "$IconPath,0"
$shortcut.Description = "Run Web applications with Go"
$shortcut.Save()

Write-Output $shortcutPath
