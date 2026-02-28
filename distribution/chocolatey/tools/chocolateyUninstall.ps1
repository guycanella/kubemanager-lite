$ErrorActionPreference = 'Stop'

$installDir   = Join-Path $env:ProgramFiles 'KubeManager Lite'
$startMenuPath = Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs'
$shortcutPath  = Join-Path $startMenuPath 'KubeManager Lite.lnk'

# Remove Start Menu shortcut
if (Test-Path $shortcutPath) {
  Remove-Item $shortcutPath -Force
  Write-Host "Removed Start Menu shortcut"
}

# Remove install directory
if (Test-Path $installDir) {
  Remove-Item $installDir -Recurse -Force
  Write-Host "Removed installation directory: $installDir"
}