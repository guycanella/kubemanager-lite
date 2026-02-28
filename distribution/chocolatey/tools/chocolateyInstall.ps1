$ErrorActionPreference = 'Stop'

$packageName  = 'kubemanager-lite'
$version      = '1.1.2'
$exeName      = 'kubemanager_lite.exe'
$installDir   = Join-Path $env:ProgramFiles 'KubeManager Lite'

$packageArgs = @{
  packageName    = $packageName
  fileType       = 'exe'
  url64bit       = "https://github.com/guycanella/kubemanager-lite/releases/download/v$version/kubemanager_lite-windows-amd64.exe"
  checksum64     = '9c3f82f4aa30b71f1ad76b57dd8b463fb19c387cfd69d83fb1617f1c9ffdfa33'
  checksumType64 = 'sha256'
  silentArgs     = ''
  validExitCodes = @(0)
  destination    = $installDir
}

# Create install directory
if (-not (Test-Path $installDir)) {
  New-Item -ItemType Directory -Path $installDir | Out-Null
}

# Download the exe to the install directory
$exePath = Join-Path $installDir $exeName
Get-ChocolateyWebFile `
  -PackageName $packageName `
  -FileFullPath $exePath `
  -Url64bit $packageArgs.url64bit `
  -Checksum64 $packageArgs.checksum64 `
  -ChecksumType64 $packageArgs.checksumType64

# Create Start Menu shortcut
$startMenuPath = Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs'
$shortcutPath  = Join-Path $startMenuPath 'KubeManager Lite.lnk'

Install-ChocolateyShortcut `
  -ShortcutFilePath $shortcutPath `
  -TargetPath $exePath `
  -Description 'KubeManager Lite — Docker and Kubernetes manager'

Write-Host "KubeManager Lite installed to $installDir"
Write-Host "Start Menu shortcut created at $shortcutPath"