param(
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\WPS Read Aloud Comate",
  [string]$ProgressFile = ""
)

$ErrorActionPreference = "Stop"
$LogDir = Join-Path $env:LOCALAPPDATA "WPSReadAloudComate\Logs"
$LogFile = Join-Path $LogDir "install.log"
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
Start-Transcript -Path $LogFile -Append | Out-Null
$AddinInternalName = "wps-read-aloud"
$AddinDisplayName = "文档朗读助手"
$AddinDescription = "WPS文档朗读助手加载项申请访问本机语音合成服务"

function Write-InstallProgress {
  param(
    [int]$Percent,
    [string]$Action,
    [string]$Detail = ""
  )
  $Percent = [Math]::Max(0, [Math]::Min(100, $Percent))
  Write-Host "$Percent% $Action $Detail"
  if ([string]::IsNullOrWhiteSpace($ProgressFile)) {
    return
  }
  try {
    $ProgressDir = Split-Path -Parent $ProgressFile
    if ($ProgressDir) {
      New-Item -ItemType Directory -Force -Path $ProgressDir | Out-Null
    }
    $Payload = [pscustomobject]@{
      percent = $Percent
      action = $Action
      detail = $Detail
      time = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss")
    } | ConvertTo-Json -Compress
    Add-Content -Path $ProgressFile -Value $Payload -Encoding UTF8
  }
  catch {
  }
}

function Backup-ConfigFile {
  param([string]$Path)
  if (Test-Path $Path) {
    $Stamp = Get-Date -Format "yyyyMMddHHmmss"
    Copy-Item -LiteralPath $Path -Destination "$Path.bak.$Stamp" -Force
  }
}

function Set-WpsPluginEntry {
  param(
    [string]$Path,
    [string]$Entry,
    [string[]]$Names
  )

  Backup-ConfigFile -Path $Path
  if (Test-Path $Path) {
    $Content = Get-Content -Raw -Path $Path -Encoding UTF8
    $Content = Remove-ProjectPluginEntries -Content $Content
    foreach ($Name in $Names) {
      $Escaped = [regex]::Escape($Name)
      $Content = [regex]::Replace($Content, "(?is)\s*<jspluginonline\b[^>]*name=`"$Escaped`"[^>]*/>", "")
      $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b[^>]*name=`"$Escaped`"[^>]*/>", "")
      $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b[^>]*name=`"$Escaped`"[\s\S]*?</jsplugin>", "")
    }
    if ($Content -match '</jsplugins>') {
      $Content = $Content -replace '(?is)</jsplugins>', "  $Entry`r`n</jsplugins>"
    }
    else {
      $Content = "<?xml version=`"1.0`" encoding=`"UTF-8`"?>`r`n<jsplugins>`r`n  $Entry`r`n</jsplugins>`r`n"
    }
  }
  else {
    $Content = "<?xml version=`"1.0`" encoding=`"UTF-8`"?>`r`n<jsplugins>`r`n  $Entry`r`n</jsplugins>`r`n"
  }
  Set-Content -Path $Path -Value $Content -Encoding UTF8
}

function Escape-XmlAttribute {
  param([string]$Value)
  return [System.Security.SecurityElement]::Escape([string]$Value)
}

function Remove-ProjectPluginEntries {
  param([string]$Content)
  $Pattern = 'wps-read-aloud|WPS Read Aloud|WPS 文档朗读助手|文档朗读助手|127\.0\.0\.1:19860'
  $Content = [regex]::Replace($Content, "(?is)\s*<jspluginonline\b(?=[^>]*($Pattern))[^>]*/>", "")
  $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b(?=[^>]*($Pattern))[^>]*/>", "")
  $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b(?=[^>]*($Pattern))[\s\S]*?</jsplugin>", "")
  return $Content
}

function Remove-WpsPluginEntry {
  param(
    [string]$Path,
    [string[]]$Names
  )
  if (!(Test-Path $Path)) {
    return
  }
  Backup-ConfigFile -Path $Path
  $Content = Get-Content -Raw -Path $Path -Encoding UTF8
  foreach ($Name in $Names) {
    $Escaped = [regex]::Escape($Name)
    $Content = [regex]::Replace($Content, "(?is)\s*<jspluginonline\b[^>]*name=`"$Escaped`"[^>]*/>", "")
    $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b[^>]*name=`"$Escaped`"[^>]*/>", "")
    $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b[^>]*name=`"$Escaped`"[\s\S]*?</jsplugin>", "")
  }
  Set-Content -Path $Path -Value $Content -Encoding UTF8
}

function Set-WpsAuthAddinEntryAllowed {
  param(
    [string]$Path,
    [string[]]$Names,
    [string]$AddinName,
    [string]$AddinPath
  )
  if (!(Test-Path $Path)) {
    return
  }
  try {
    Backup-ConfigFile -Path $Path
    $Data = Get-Content -Raw -Path $Path -Encoding UTF8 | ConvertFrom-Json
    if (!$Data.wps) {
      return
    }
    $Changed = $false
    foreach ($Property in $Data.wps.PSObject.Properties) {
      if ($Property.Name -eq "namelist") {
        continue
      }
      $Value = $Property.Value
      $KnownName = $false
      foreach ($Name in $Names) {
        if ($Value.name -eq $Name) {
          $KnownName = $true
          break
        }
      }
      $KnownPath = ($Value.path -match '127\.0\.0\.1:19860/addin')
      if ($KnownName -or $KnownPath) {
        $Value.name = $AddinName
        $Value.path = $AddinPath
        $Value.enable = $true
        $Value.isload = $true
        if (!$Value.mode) {
          $Value | Add-Member -NotePropertyName "mode" -NotePropertyValue 2 -Force
        }
        $Changed = $true
      }
    }
    if ($Changed) {
      $Data | ConvertTo-Json -Depth 10 | Set-Content -Path $Path -Encoding UTF8
      Write-Host "已保留并刷新 WPS 加载项授权缓存，避免升级后重复确认。"
    }
  }
  catch {
    Write-Host "WPS 加载项授权缓存刷新失败，已跳过：$($_.Exception.Message)"
  }
}

function Clear-WpsJsAddinBlockHost {
  param([string]$JsDir)
  $BlockFile = Join-Path $JsDir "jsaddinblockhost.ini"
  if (!(Test-Path $BlockFile)) {
    return
  }
  try {
    Backup-ConfigFile -Path $BlockFile
    Remove-Item -LiteralPath $BlockFile -Force
    Write-Host "已清理 WPS JS 加载项阻止缓存：$BlockFile"
  }
  catch {
    Write-Host "WPS JS 加载项阻止缓存清理失败，已跳过：$($_.Exception.Message)"
  }
}

function Stop-InstalledDaemonProcess {
  param([string]$Root)
  if ([string]::IsNullOrWhiteSpace($Root) -or !(Test-Path $Root)) {
    return
  }
  $FullRoot = [System.IO.Path]::GetFullPath($Root)
  Get-Process -Name "wps-tts-daemon" -ErrorAction SilentlyContinue |
    ForEach-Object {
      try {
        $Path = $_.Path
        if ($Path -and [System.IO.Path]::GetFullPath($Path).StartsWith($FullRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
          Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
        }
      }
      catch {
      }
    }
}

function Clear-InstalledPayload {
  param([string]$Root)
  if ([string]::IsNullOrWhiteSpace($Root) -or !(Test-Path $Root)) {
    return
  }
  $KnownNames = @(
    "addin",
    "daemon",
    "engines",
    "installer-assets",
    "third_party_licenses",
    "voices",
    "ACCEPTANCE_TEST.md",
    "RELEASE_NOTES.md",
    "SOURCE_OFFER.md",
    "config.yaml",
    "install-state.json",
    "start-daemon.ps1",
    "uninstall.ps1",
    "version.json"
  )
  foreach ($Name in $KnownNames) {
    Remove-Item -LiteralPath (Join-Path $Root $Name) -Recurse -Force -ErrorAction SilentlyContinue
  }
  Get-ChildItem -LiteralPath $Root -Filter "*.7z" -File -ErrorAction SilentlyContinue |
    Remove-Item -Force -ErrorAction SilentlyContinue
}

function Get-PeArchitecture {
  param([string]$Path)
  if (!(Test-Path $Path)) {
    return "unknown"
  }
  $Stream = [System.IO.File]::Open($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
  try {
    $Reader = New-Object System.IO.BinaryReader($Stream)
    $Stream.Seek(0x3c, [System.IO.SeekOrigin]::Begin) | Out-Null
    $PeOffset = $Reader.ReadInt32()
    $Stream.Seek($PeOffset + 4, [System.IO.SeekOrigin]::Begin) | Out-Null
    $Machine = $Reader.ReadUInt16()
    switch ($Machine) {
      0x014c { return "x86" }
      0x8664 { return "x64" }
      0xaa64 { return "arm64" }
      default { return "unknown" }
    }
  }
  finally {
    $Stream.Close()
  }
}

function Get-WpsVersionNumber {
  param([string]$VersionText)
  if ([string]::IsNullOrWhiteSpace($VersionText)) {
    return $null
  }
  $Match = [regex]::Match($VersionText, '\d+(\.\d+){0,3}')
  if (!$Match.Success) {
    return $null
  }
  try {
    return [version]$Match.Value
  }
  catch {
    return $null
  }
}

function Add-WpsCandidate {
  param(
    [System.Collections.Generic.List[string]]$Candidates,
    [string]$Value
  )

  if ([string]::IsNullOrWhiteSpace($Value)) {
    return
  }
  $Clean = $Value.Trim()
  $Clean = $Clean -replace '^"|"$', ''
  $Clean = $Clean -replace ',\d+$', ''
  if ($Clean -match '^"([^"]+)"') {
    $Clean = $Matches[1]
  }
  if ($Clean -match '^(.*?\.exe)\b') {
    $Clean = $Matches[1]
  }
  $Expanded = [Environment]::ExpandEnvironmentVariables($Clean)
  if ([string]::IsNullOrWhiteSpace($Expanded)) {
    return
  }
  if (Test-Path $Expanded -PathType Leaf) {
    if ([System.IO.Path]::GetFileName($Expanded) -ieq "wps.exe") {
      $Candidates.Add($Expanded)
    }
    return
  }
  if (Test-Path $Expanded -PathType Container) {
    $Direct = Join-Path $Expanded "wps.exe"
    if (Test-Path $Direct -PathType Leaf) {
      $Candidates.Add($Direct)
      return
    }
    Get-ChildItem -Path $Expanded -Filter "wps.exe" -Recurse -ErrorAction SilentlyContinue |
      ForEach-Object { $Candidates.Add($_.FullName) }
  }
}

function Add-WpsShortcutCandidates {
  param([System.Collections.Generic.List[string]]$Candidates)

  $Shell = $null
  try {
    $Shell = New-Object -ComObject WScript.Shell
  }
  catch {
    return
  }
  $ShortcutRoots = @(
    "$env:APPDATA\Microsoft\Windows\Start Menu\Programs",
    "$env:ProgramData\Microsoft\Windows\Start Menu\Programs",
    "$env:USERPROFILE\Desktop",
    "$env:PUBLIC\Desktop"
  )
  foreach ($Root in $ShortcutRoots) {
    if (!(Test-Path $Root)) {
      continue
    }
    Get-ChildItem -Path $Root -Filter "*.lnk" -Recurse -ErrorAction SilentlyContinue |
      Where-Object { $_.Name -match 'WPS|Kingsoft|金山' } |
      ForEach-Object {
        try {
          $Shortcut = $Shell.CreateShortcut($_.FullName)
          Add-WpsCandidate -Candidates $Candidates -Value $Shortcut.TargetPath
          Add-WpsCandidate -Candidates $Candidates -Value $Shortcut.WorkingDirectory
        }
        catch {
        }
      }
  }
}

function Find-WpsInstallations {
  $Candidates = New-Object System.Collections.Generic.List[string]
  $AppPathRoots = @(
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\App Paths\wps.exe",
    "HKLM:\Software\Microsoft\Windows\CurrentVersion\App Paths\wps.exe",
    "HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths\wps.exe"
  )
  foreach ($Root in $AppPathRoots) {
    $Item = Get-ItemProperty -Path $Root -ErrorAction SilentlyContinue
    if ($Item) {
      Add-WpsCandidate -Candidates $Candidates -Value $Item.'(default)'
      Add-WpsCandidate -Candidates $Candidates -Value $Item.Path
    }
  }

  $RegistryRoots = @(
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*",
    "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*",
    "HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*"
  )
  foreach ($Root in $RegistryRoots) {
    Get-ItemProperty -Path $Root -ErrorAction SilentlyContinue |
      Where-Object { $_.DisplayName -match 'WPS|Kingsoft|金山' } |
      ForEach-Object {
        foreach ($Value in @($_.InstallLocation, $_.DisplayIcon)) {
          Add-WpsCandidate -Candidates $Candidates -Value $Value
        }
      }
  }

  $KingsoftRoots = @(
    "HKCU:\Software\Kingsoft\Office",
    "HKCU:\Software\Kingsoft\Office\*",
    "HKCU:\Software\Kingsoft\WPS",
    "HKCU:\Software\Kingsoft\WPS\*",
    "HKLM:\Software\Kingsoft\Office",
    "HKLM:\Software\Kingsoft\Office\*",
    "HKLM:\Software\Kingsoft\WPS",
    "HKLM:\Software\Kingsoft\WPS\*",
    "HKLM:\Software\WOW6432Node\Kingsoft\Office",
    "HKLM:\Software\WOW6432Node\Kingsoft\Office\*",
    "HKLM:\Software\WOW6432Node\Kingsoft\WPS",
    "HKLM:\Software\WOW6432Node\Kingsoft\WPS\*"
  )
  $PathPropertyNames = @(
    "InstallRoot", "InstallPath", "InstallLocation", "Path", "ProgramPath",
    "OfficePath", "RootPath", "Home", "ProductPath", "ExePath", "wps"
  )
  foreach ($Root in $KingsoftRoots) {
    Get-ItemProperty -Path $Root -ErrorAction SilentlyContinue | ForEach-Object {
      foreach ($Name in $PathPropertyNames) {
        if ($_.PSObject.Properties.Name -contains $Name) {
          Add-WpsCandidate -Candidates $Candidates -Value $_.$Name
        }
      }
    }
  }

  $Roots = @(
    "$env:ProgramW6432\Kingsoft\WPS Office",
    "$env:ProgramW6432\WPS Office",
    "$env:ProgramFiles\Kingsoft\WPS Office",
    "${env:ProgramFiles(x86)}\Kingsoft\WPS Office",
    "$env:ProgramFiles\WPS Office",
    "${env:ProgramFiles(x86)}\WPS Office",
    "C:\Program Files\Kingsoft\WPS Office",
    "C:\Program Files (x86)\Kingsoft\WPS Office",
    "C:\Program Files\WPS Office",
    "C:\Program Files (x86)\WPS Office",
    "$env:LOCALAPPDATA\Kingsoft\WPS Office",
    "$env:LOCALAPPDATA\WPS Office",
    "$env:LOCALAPPDATA\Kingsoft",
    "$env:LOCALAPPDATA\WPS",
    "$env:APPDATA\Kingsoft",
    "$env:APPDATA\WPS"
  )
  foreach ($Root in $Roots) {
    Add-WpsCandidate -Candidates $Candidates -Value $Root
  }
  Add-WpsShortcutCandidates -Candidates $Candidates
  Get-Command "wps.exe" -ErrorAction SilentlyContinue |
    ForEach-Object { Add-WpsCandidate -Candidates $Candidates -Value $_.Source }

  $Candidates |
    Where-Object { $_ -and (Test-Path $_) -and ([System.IO.Path]::GetFileName($_) -ieq "wps.exe") } |
    Sort-Object -Unique |
    ForEach-Object {
      $Info = [System.Diagnostics.FileVersionInfo]::GetVersionInfo($_)
      [pscustomobject]@{
        Path = $_
        Directory = Split-Path -Parent $_
        Architecture = Get-PeArchitecture -Path $_
        VersionText = $Info.ProductVersion
        FileVersion = $Info.FileVersion
        ProductName = $Info.ProductName
        Version = Get-WpsVersionNumber -VersionText $Info.ProductVersion
      }
    }
}

function Test-WpsRequirement {
  param(
    [object[]]$Installations,
    [string]$PackageArch
  )
  if (!$Installations -or $Installations.Count -eq 0) {
    throw "未检测到 WPS Office。请先安装 WPS Office 2019 或更高版本，再运行本安装包。"
  }

  $Supported = @()
  foreach ($Item in $Installations) {
    $VersionOk = ($null -eq $Item.Version) -or ($Item.Version.Major -ge 11)
    if ($VersionOk) {
      $Supported += $Item
    }
  }
  if ($Supported.Count -gt 0) {
    return ($Supported | Sort-Object Path | Select-Object -First 1)
  }

  $Detected = ($Installations | ForEach-Object {
    "路径：$($_.Path)；位数：$($_.Architecture)；版本：$($_.VersionText)"
  }) -join "`r`n"
  throw "未找到符合版本要求的 WPS Office。x86/x64 Windows 10/11 最低要求 WPS Office 2019 或更高版本。`r`n已检测到：`r`n$Detected`r`n请升级或安装 WPS Office 后再运行本安装包。"
}

function New-DaemonLauncher {
  param(
    [string]$Root,
    [string]$Daemon,
    [string]$Config
  )

  $Launcher = Join-Path $Root "start-daemon.ps1"
  $Content = @"
`$ErrorActionPreference = "SilentlyContinue"
`$Root = Split-Path -Parent `$MyInvocation.MyCommand.Path
`$Daemon = Join-Path `$Root "daemon\wps-tts-daemon.exe"
`$Config = Join-Path `$Root "config.yaml"
if (!(Test-Path `$Daemon)) {
  exit 0
}
`$Healthy = `$false
try {
  `$Response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:19860/health" -TimeoutSec 2
  `$Healthy = (`$Response.StatusCode -ge 200 -and `$Response.StatusCode -lt 500)
}
catch {
  `$Healthy = `$false
}
if (!`$Healthy) {
  Start-Process -FilePath `$Daemon -ArgumentList @("-config", `$Config) -WorkingDirectory `$Root -WindowStyle Hidden
}
"@
  Set-Content -Path $Launcher -Value $Content -Encoding UTF8
  return $Launcher
}

function ConvertTo-FileUri {
  param([string]$Path)
  return ([System.Uri]([System.IO.Path]::GetFullPath($Path))).AbsoluteUri
}

function Remove-WpsOemProjectConfig {
  param([object]$WpsInfo)
  $OemPath = Join-Path $WpsInfo.Directory "cfgs\oem.ini"
  if (!(Test-Path $OemPath)) {
    return
  }
  Backup-ConfigFile -Path $OemPath
  $Lines = New-Object System.Collections.Generic.List[string]
  foreach ($Line in (Get-Content -Path $OemPath -Encoding UTF8)) {
    if ($Line -match '^\s*JSPluginsServer\s*=' -and ($Line -match 'Kingsoft[\\/]+wps[\\/]+jsaddons[\\/]+jsplugins\.xml' -or $Line -match '127\.0\.0\.1:19860')) {
      continue
    }
    $Lines.Add($Line)
  }
  Set-Content -Path $OemPath -Value ($Lines -join "`r`n") -Encoding UTF8
  Write-Host "已清理本项目旧版 WPS OEM 指向项：$OemPath"
}

function Write-WindowsRuntimeConfig {
  param(
    [string]$Path,
    [string]$Root,
    [string]$Launcher,
    [string]$Daemon,
    [string]$Config
  )
  $Docs = ConvertTo-FileUri -Path $Root
  $Payload = [ordered]@{
    platform = "windows"
    serviceOrigin = "http://127.0.0.1:19860"
    installRoot = $Root
    launcherPath = $Launcher
    daemonExe = $Daemon
    configPath = $Config
    docsBaseUrl = $Docs.TrimEnd("/") + "/"
  } | ConvertTo-Json -Depth 5 -Compress
  $Content = "window.WPS_READ_ALOUD_RUNTIME = $Payload;`r`n"
  Set-Content -Path $Path -Value $Content -Encoding UTF8
}

function Remove-ProjectStartupEntries {
  Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "WPSReadAloudComate" -ErrorAction SilentlyContinue
  try {
    Stop-ScheduledTask -TaskName "WPSReadAloudComate" -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName "WPSReadAloudComate" -Confirm:$false -ErrorAction SilentlyContinue
  }
  catch {
    Write-Host "旧版计划任务清理被系统拒绝，已跳过。"
  }
}

function Register-DaemonStartup {
  param([string]$Launcher)
  $PowerShell = Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"
  if (!(Test-Path $PowerShell)) {
    $PowerShell = "powershell.exe"
  }
  $Command = "`"$PowerShell`" -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$Launcher`""
  New-Item -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Force | Out-Null
  Set-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "WPSReadAloudComate" -Value $Command
  Write-Host "已写入当前用户开机自启动项：WPSReadAloudComate"
}

function Start-DaemonNow {
  param(
    [string]$Root,
    [string]$Daemon,
    [string]$Config
  )
  $Healthy = $false
  try {
    $Response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:19860/health" -TimeoutSec 2
    $Healthy = ($Response.StatusCode -ge 200 -and $Response.StatusCode -lt 500)
  }
  catch {
    $Healthy = $false
  }
  if ($Healthy) {
    return
  }
  if (!(Test-Path $Daemon)) {
    throw "安装包不完整：未找到本机朗读服务程序。"
  }
  Start-Process -FilePath $Daemon -ArgumentList @("-config", $Config) -WorkingDirectory $Root -WindowStyle Hidden | Out-Null
}

function Wait-LocalServiceHealthy {
  param([int]$TimeoutSeconds = 20)
  $Deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $Deadline) {
    try {
      $Response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:19860/health" -TimeoutSec 2
      if ($Response.StatusCode -ge 200 -and $Response.StatusCode -lt 500) {
        return $true
      }
    }
    catch {
    }
    Start-Sleep -Milliseconds 500
  }
  return $false
}

function New-Shortcut {
  param(
    [object]$Shell,
    [string]$Path,
    [string]$TargetPath,
    [string]$Arguments,
    [string]$WorkingDirectory,
    [string]$IconPath
  )
  $ShortcutDir = Split-Path -Parent $Path
  New-Item -ItemType Directory -Force -Path $ShortcutDir | Out-Null
  $Shortcut = $Shell.CreateShortcut($Path)
  $Shortcut.TargetPath = $TargetPath
  $Shortcut.Arguments = $Arguments
  $Shortcut.WorkingDirectory = $WorkingDirectory
  if (Test-Path $IconPath) {
    $Shortcut.IconLocation = $IconPath
  }
  $Shortcut.Save()
}

function Test-IsAdministrator {
  try {
    $Identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $Principal = New-Object Security.Principal.WindowsPrincipal($Identity)
    return $Principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  }
  catch {
    return $false
  }
}

function Clear-StartMenuEntries {
  $Roots = @(
    [Environment]::GetFolderPath("Programs"),
    [Environment]::GetFolderPath("CommonPrograms")
  )
  foreach ($Root in $Roots) {
    if ([string]::IsNullOrWhiteSpace($Root)) {
      continue
    }
    foreach ($Name in @("WPS文档朗读助手", "WPS 文档朗读助手")) {
      Remove-Item -LiteralPath (Join-Path $Root $Name) -Recurse -Force -ErrorAction SilentlyContinue
    }
    Remove-Item -LiteralPath (Join-Path $Root "卸载 WPS文档朗读助手.lnk") -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $Root "卸载 WPS 文档朗读助手.lnk") -Force -ErrorAction SilentlyContinue
  }
}

function New-StartMenuShortcuts {
  param([string]$Root)
  $PowerShell = Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"
  if (!(Test-Path $PowerShell)) {
    $PowerShell = "powershell.exe"
  }
  $Explorer = Join-Path $env:WINDIR "explorer.exe"
  $Shell = New-Object -ComObject WScript.Shell
  $IconPath = Join-Path $Root "installer-assets\app.ico"
  $UninstallArguments = "-NoProfile -ExecutionPolicy Bypass -File `"$Root\uninstall.ps1`""
  $Folders = @()
  $UserPrograms = [Environment]::GetFolderPath("Programs")
  if (![string]::IsNullOrWhiteSpace($UserPrograms)) {
    $Folders += (Join-Path $UserPrograms "WPS文档朗读助手")
  }
  $CommonPrograms = [Environment]::GetFolderPath("CommonPrograms")
  if ((Test-IsAdministrator) -and ![string]::IsNullOrWhiteSpace($CommonPrograms)) {
    $Folders += (Join-Path $CommonPrograms "WPS文档朗读助手")
  }
  foreach ($Folder in ($Folders | Sort-Object -Unique)) {
    try {
      New-Item -ItemType Directory -Force -Path $Folder | Out-Null
      $OpenPath = Join-Path $Folder "打开安装目录.lnk"
      $UninstallPath = Join-Path $Folder "卸载 WPS文档朗读助手.lnk"
      New-Shortcut -Shell $Shell -Path $OpenPath -TargetPath $Explorer -Arguments "`"$Root`"" -WorkingDirectory $Root -IconPath $IconPath
      New-Shortcut -Shell $Shell -Path $UninstallPath -TargetPath $PowerShell -Arguments $UninstallArguments -WorkingDirectory $Root -IconPath $IconPath
      Write-Host "已创建开始菜单文件夹：$Folder"
    }
    catch {
      Write-Host "开始菜单入口创建失败，已跳过：$($_.Exception.Message)"
    }
  }
}

function Register-UninstallEntry {
  param(
    [string]$Root,
    [object]$VersionInfo
  )
  $Key = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\WPSReadAloudComate"
  New-Item -Path $Key -Force | Out-Null
  $PowerShell = Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"
  if (!(Test-Path $PowerShell)) {
    $PowerShell = "powershell.exe"
  }
  $UninstallScript = Join-Path $Root "uninstall.ps1"
  $DisplayIcon = Join-Path $Root "installer-assets\app.ico"
  if (!(Test-Path $DisplayIcon)) {
    $DisplayIcon = Join-Path $Root "daemon\wps-tts-daemon.exe"
  }
  Set-ItemProperty -Path $Key -Name "DisplayName" -Value "WPS 文档朗读助手"
  Set-ItemProperty -Path $Key -Name "DisplayVersion" -Value ([string]$VersionInfo.version)
  Set-ItemProperty -Path $Key -Name "Publisher" -Value "Zhang Jingyao"
  Set-ItemProperty -Path $Key -Name "InstallLocation" -Value $Root
  Set-ItemProperty -Path $Key -Name "DisplayIcon" -Value $DisplayIcon
  Set-ItemProperty -Path $Key -Name "UninstallString" -Value "`"$PowerShell`" -NoProfile -ExecutionPolicy Bypass -File `"$UninstallScript`""
  Set-ItemProperty -Path $Key -Name "QuietUninstallString" -Value "`"$PowerShell`" -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$UninstallScript`" -Silent"
  Set-ItemProperty -Path $Key -Name "NoModify" -Type DWord -Value 1
  Set-ItemProperty -Path $Key -Name "NoRepair" -Type DWord -Value 1
  try {
    $SizeKb = [int]((Get-ChildItem -LiteralPath $Root -Recurse -File -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum / 1KB)
    Set-ItemProperty -Path $Key -Name "EstimatedSize" -Type DWord -Value $SizeKb
  }
  catch {
  }
}

function Write-InstallState {
  param(
    [string]$Root,
    [object]$WpsInfo,
    [string]$PluginsXml
  )
  $State = [ordered]@{
    wpsExe = [string]$WpsInfo.Path
    wpsOemIni = [string](Join-Path $WpsInfo.Directory "cfgs\oem.ini")
    jsPluginsXml = [string]$PluginsXml
    jsPluginsServer = [string](ConvertTo-FileUri -Path $PluginsXml)
  } | ConvertTo-Json -Depth 4
  Set-Content -Path (Join-Path $Root "install-state.json") -Value $State -Encoding UTF8
}

try {
  Write-InstallProgress -Percent 3 -Action "初始化安装" -Detail "正在准备安装环境"
  $Source = Join-Path $PSScriptRoot "app"
  if (!(Test-Path $Source)) {
    throw "安装包不完整：未找到 app 目录。"
  }
  Write-InstallProgress -Percent 10 -Action "检测 WPS" -Detail "正在查找本机 WPS Office"
  $VersionInfo = Get-Content -Raw -Path (Join-Path $Source "version.json") | ConvertFrom-Json
  $PackageArch = $VersionInfo.architecture
  if ($PackageArch -eq "386") {
    $PackageArch = "x86"
  }
  $WpsInfo = Test-WpsRequirement -Installations (Find-WpsInstallations) -PackageArch $PackageArch
  Write-Host "检测到 WPS：$($WpsInfo.Path)"
  Write-Host "WPS 位数：$($WpsInfo.Architecture)；WPS 版本：$($WpsInfo.VersionText)"
  Write-Host "本安装包使用独立本地朗读服务，不注入 WPS 进程，可同时支持 32 位和 64 位 WPS。"

  Write-InstallProgress -Percent 20 -Action "清理旧版本" -Detail "正在停止旧版朗读服务"
  Remove-ProjectStartupEntries
  Stop-InstalledDaemonProcess -Root $InstallDir
  Write-InstallProgress -Percent 35 -Action "复制程序文件" -Detail "安装路径：$InstallDir"
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  Clear-InstalledPayload -Root $InstallDir
  Copy-Item -Path (Join-Path $Source "*") -Destination $InstallDir -Recurse -Force
  Copy-Item -LiteralPath (Join-Path $PSScriptRoot "uninstall.ps1") -Destination (Join-Path $InstallDir "uninstall.ps1") -Force
  if (Test-Path (Join-Path $PSScriptRoot "installer-assets")) {
    $InstallAssets = Join-Path $InstallDir "installer-assets"
    Remove-Item -LiteralPath $InstallAssets -Recurse -Force -ErrorAction SilentlyContinue
    Copy-Item -Path (Join-Path $PSScriptRoot "installer-assets") -Destination $InstallAssets -Recurse -Force
  }

  $Daemon = Join-Path $InstallDir "daemon\wps-tts-daemon.exe"
  if (!(Test-Path $Daemon)) {
    throw "安装包不完整：未找到 wps-tts-daemon.exe。"
  }

  $Launcher = New-DaemonLauncher -Root $InstallDir -Daemon $Daemon -Config (Join-Path $InstallDir "config.yaml")
  Write-InstallProgress -Percent 55 -Action "配置本机服务" -Detail "正在注册并启动本机朗读服务"
  Register-DaemonStartup -Launcher $Launcher
  Start-DaemonNow -Root $InstallDir -Daemon $Daemon -Config (Join-Path $InstallDir "config.yaml")
  if (!(Wait-LocalServiceHealthy -TimeoutSeconds 25)) {
    throw "本机朗读服务启动失败。请确认 127.0.0.1:19860 未被其他程序占用，并查看安装日志。"
  }
  Write-Host "本机朗读服务已启动，并会随当前用户登录自动启动。"

  Write-InstallProgress -Percent 70 -Action "注册 WPS 加载项" -Detail "正在写入 WPS publish 加载项配置"
  $JsDir = Join-Path $env:APPDATA "Kingsoft\wps\jsaddons"
  New-Item -ItemType Directory -Force -Path $JsDir | Out-Null
  $AddinVersion = $VersionInfo.version
  Get-ChildItem -Path $JsDir -Directory -Filter "wps-read-aloud_*" -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
  Get-ChildItem -Path $JsDir -Directory -Filter "$AddinDisplayName`_*" -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
  Write-WindowsRuntimeConfig -Path (Join-Path $InstallDir "addin\assets\runtime-config.js") -Root $InstallDir -Launcher $Launcher -Daemon $Daemon -Config (Join-Path $InstallDir "config.yaml")

  $PublishXml = Join-Path $JsDir "publish.xml"
  $PluginsXml = Join-Path $JsDir "jsplugins.xml"
  $KnownNames = @($AddinInternalName, $AddinDisplayName, "WPS 文档朗读助手", "wps-read-aloud-comate", "wps-read-aloud-xc", "wps-read-aloud-zhangjingyao")
  $LocalUrl = "http://127.0.0.1:19860/addin/"
  $PublishEntry = '<jspluginonline name="' + (Escape-XmlAttribute $AddinDisplayName) + '" type="wps" enable="enable_dev" install="' + (Escape-XmlAttribute $LocalUrl) + '" url="' + (Escape-XmlAttribute $LocalUrl) + '" desc="' + (Escape-XmlAttribute $AddinDescription) + '"/>'
  $OnlineEntry = '<jsplugin name="' + (Escape-XmlAttribute $AddinDisplayName) + '" type="wps" enable="enable_dev" url="' + (Escape-XmlAttribute $LocalUrl) + '" version="' + (Escape-XmlAttribute $AddinVersion) + '" desc="' + (Escape-XmlAttribute $AddinDescription) + '"><ribbon file="' + (Escape-XmlAttribute ($LocalUrl + "ribbon.xml")) + '"/></jsplugin>'
  Set-WpsPluginEntry -Path $PublishXml -Entry $PublishEntry -Names $KnownNames
  Set-WpsPluginEntry -Path $PluginsXml -Entry $OnlineEntry -Names $KnownNames
  Remove-WpsOemProjectConfig -WpsInfo $WpsInfo
  Set-WpsAuthAddinEntryAllowed -Path (Join-Path $JsDir "authaddin.json") -Names $KnownNames -AddinName $AddinDisplayName -AddinPath $LocalUrl
  Clear-WpsJsAddinBlockHost -JsDir $JsDir
  Write-InstallState -Root $InstallDir -WpsInfo $WpsInfo -PluginsXml $PluginsXml
  Write-Host "已写入 WPS publish 加载项地址：$LocalUrl"
  Write-InstallProgress -Percent 86 -Action "注册卸载入口" -Detail "正在写入开始菜单和控制面板卸载信息"
  Clear-StartMenuEntries
  New-StartMenuShortcuts -Root $InstallDir
  Register-UninstallEntry -Root $InstallDir -VersionInfo $VersionInfo

  Write-InstallProgress -Percent 100 -Action "安装完成" -Detail "请彻底退出并重新打开 WPS"
  Write-Host "WPS 文档朗读助手安装完成。请彻底退出并重新打开 WPS，在顶部查看“文档朗读”选项卡。"
  Write-Host "安装日志：$LogFile"
}
catch {
  Write-InstallProgress -Percent 100 -Action "安装失败" -Detail $_.Exception.Message
  throw
}
finally {
  Stop-Transcript | Out-Null
}
