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

function New-OfflineAddinPackage {
  param(
    [string]$JsDir,
    [string]$FolderName,
    [string]$OutputPath
  )

  $SourceDir = Join-Path $JsDir $FolderName
  if (!(Test-Path $SourceDir -PathType Container)) {
    throw "安装包不完整：未找到离线加载项目录 $SourceDir。"
  }
  $Tar = Get-Command "tar.exe" -ErrorAction SilentlyContinue
  if (!$Tar) {
    throw "系统缺少 tar.exe，无法生成 WPS 离线加载项包。请确认系统为 Windows 10/11。"
  }
  Remove-Item -LiteralPath $OutputPath -Force -ErrorAction SilentlyContinue
  $OutputDir = Split-Path -Parent $OutputPath
  if ($OutputDir) {
    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
  }
  & $Tar.Source -a -cf $OutputPath -C $JsDir $FolderName
  if ($LASTEXITCODE -ne 0 -or !(Test-Path $OutputPath -PathType Leaf)) {
    throw "生成 WPS 离线加载项包失败。"
  }
  $Stream = [System.IO.File]::OpenRead($OutputPath)
  try {
    $Header = New-Object byte[] 6
    $Read = $Stream.Read($Header, 0, $Header.Length)
    $Expected = [byte[]](0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c)
    if ($Read -ne 6) {
      throw "生成的离线加载项包为空。"
    }
    for ($i = 0; $i -lt 6; $i += 1) {
      if ($Header[$i] -ne $Expected[$i]) {
        throw "生成的离线加载项包不是有效的 7z 格式。"
      }
    }
  }
  finally {
    $Stream.Close()
  }
}

function Set-IniSectionValue {
  param(
    [string]$Content,
    [string]$Section,
    [string]$Key,
    [string]$Value
  )
  $Lines = New-Object System.Collections.Generic.List[string]
  if (![string]::IsNullOrEmpty($Content)) {
    foreach ($Line in ($Content -split "`r?`n")) {
      $Lines.Add($Line)
    }
  }
  $SectionHeader = "[$Section]"
  $SectionIndex = -1
  for ($i = 0; $i -lt $Lines.Count; $i += 1) {
    if ($Lines[$i].Trim().Equals($SectionHeader, [System.StringComparison]::OrdinalIgnoreCase)) {
      $SectionIndex = $i
      break
    }
  }
  if ($SectionIndex -lt 0) {
    if ($Lines.Count -gt 0 -and ![string]::IsNullOrWhiteSpace($Lines[$Lines.Count - 1])) {
      $Lines.Add("")
    }
    $Lines.Add($SectionHeader)
    $Lines.Add("$Key=$Value")
    return ($Lines -join "`r`n")
  }
  $InsertAt = $Lines.Count
  for ($i = $SectionIndex + 1; $i -lt $Lines.Count; $i += 1) {
    if ($Lines[$i].Trim() -match '^\[[^\]]+\]$') {
      $InsertAt = $i
      break
    }
    if ($Lines[$i] -match "^\s*$([regex]::Escape($Key))\s*=") {
      $Lines[$i] = "$Key=$Value"
      return ($Lines -join "`r`n")
    }
  }
  $Lines.Insert($InsertAt, "$Key=$Value")
  return ($Lines -join "`r`n")
}

function Set-WpsOemOfflineConfig {
  param(
    [object]$WpsInfo,
    [string]$PluginsXml
  )
  $OemPath = Join-Path $WpsInfo.Directory "cfgs\oem.ini"
  $OemDir = Split-Path -Parent $OemPath
  if (!(Test-Path $OemDir)) {
    New-Item -ItemType Directory -Force -Path $OemDir | Out-Null
  }
  if (!(Test-Path $OemPath)) {
    New-Item -ItemType File -Force -Path $OemPath | Out-Null
  }
  Backup-ConfigFile -Path $OemPath
  $Content = Get-Content -Raw -Path $OemPath -Encoding UTF8
  $ServerUri = ConvertTo-FileUri -Path $PluginsXml
  $Content = Set-IniSectionValue -Content $Content -Section "Support" -Key "JsApiPlugin" -Value "true"
  $Content = Set-IniSectionValue -Content $Content -Section "Support" -Key "disableFileCheckIntercept" -Value "true"
  $Content = Set-IniSectionValue -Content $Content -Section "Server" -Key "JSPluginsServer" -Value $ServerUri
  Set-Content -Path $OemPath -Value $Content -Encoding UTF8
  Write-Host "已更新 WPS OEM 配置：$OemPath"
  Write-Host "JSPluginsServer：$ServerUri"
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

function Remove-OldStartupEntries {
  Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "WPSReadAloudComate" -ErrorAction SilentlyContinue
  try {
    Stop-ScheduledTask -TaskName "WPSReadAloudComate" -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName "WPSReadAloudComate" -Confirm:$false -ErrorAction SilentlyContinue
  }
  catch {
    Write-Host "旧版计划任务清理被系统拒绝，已跳过。"
  }
}

function New-UninstallShortcut {
  param([string]$Root)
  $ShortcutDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\WPS文档朗读助手"
  New-Item -ItemType Directory -Force -Path $ShortcutDir | Out-Null
  $ShortcutPath = Join-Path $ShortcutDir "卸载 WPS文档朗读助手.lnk"
  $PowerShell = Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"
  if (!(Test-Path $PowerShell)) {
    $PowerShell = "powershell.exe"
  }
  $Shell = New-Object -ComObject WScript.Shell
  $Shortcut = $Shell.CreateShortcut($ShortcutPath)
  $Shortcut.TargetPath = $PowerShell
  $Shortcut.Arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$Root\uninstall.ps1`""
  $Shortcut.WorkingDirectory = $Root
  $IconPath = Join-Path $Root "installer-assets\app.ico"
  if (Test-Path $IconPath) {
    $Shortcut.IconLocation = $IconPath
  }
  $Shortcut.Save()
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
  Remove-OldStartupEntries
  Stop-InstalledDaemonProcess -Root $InstallDir
  Write-InstallProgress -Percent 35 -Action "复制程序文件" -Detail "安装路径：$InstallDir"
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
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
  Write-InstallProgress -Percent 55 -Action "配置本机服务" -Detail "朗读服务不写入开机自启动项"
  Write-Host "已清理旧版自启动项。新版 Windows 包不会开机自启动朗读服务。"

  Write-InstallProgress -Percent 70 -Action "注册 WPS 加载项" -Detail "正在写入 WPS 离线加载项配置"
  $JsDir = Join-Path $env:APPDATA "Kingsoft\wps\jsaddons"
  New-Item -ItemType Directory -Force -Path $JsDir | Out-Null
  $AddinVersion = $VersionInfo.version
  Get-ChildItem -Path $JsDir -Directory -Filter "wps-read-aloud_*" -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
  Get-ChildItem -Path $JsDir -Directory -Filter "$AddinDisplayName`_*" -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
  $OfflineFolderName = $AddinDisplayName + "_" + $AddinVersion
  $Target = Join-Path $JsDir $OfflineFolderName
  New-Item -ItemType Directory -Force -Path $Target | Out-Null
  Copy-Item -Path (Join-Path $InstallDir "addin\*") -Destination $Target -Recurse -Force
  Write-WindowsRuntimeConfig -Path (Join-Path $Target "assets\runtime-config.js") -Root $InstallDir -Launcher $Launcher -Daemon $Daemon -Config (Join-Path $InstallDir "config.yaml")
  Write-WindowsRuntimeConfig -Path (Join-Path $InstallDir "addin\assets\runtime-config.js") -Root $InstallDir -Launcher $Launcher -Daemon $Daemon -Config (Join-Path $InstallDir "config.yaml")
  $OfflinePackagePath = Join-Path $InstallDir ($OfflineFolderName + ".7z")
  New-OfflineAddinPackage -JsDir $JsDir -FolderName $OfflineFolderName -OutputPath $OfflinePackagePath

  $PublishXml = Join-Path $JsDir "publish.xml"
  $PluginsXml = Join-Path $JsDir "jsplugins.xml"
  $KnownNames = @($AddinInternalName, $AddinDisplayName, "WPS 文档朗读助手", "wps-read-aloud-comate", "wps-read-aloud-xc", "wps-read-aloud-zhangjingyao")
  Remove-WpsPluginEntry -Path $PublishXml -Names $KnownNames
  $OfflinePackageUrl = (ConvertTo-FileUri -Path $OfflinePackagePath)
  $OfflineEntry = '<jsplugin name="' + (Escape-XmlAttribute $AddinDisplayName) + '" type="wps" version="' + (Escape-XmlAttribute $AddinVersion) + '" url="' + (Escape-XmlAttribute $OfflinePackageUrl) + '" desc="' + (Escape-XmlAttribute $AddinDescription) + '"/>'
  Set-WpsPluginEntry -Path $PluginsXml -Entry $OfflineEntry -Names $KnownNames
  try {
    Set-WpsOemOfflineConfig -WpsInfo $WpsInfo -PluginsXml $PluginsXml
  }
  catch {
    throw "写入 WPS OEM 配置失败。publish 离线模式需要修改 WPS 安装目录下的 office6\cfgs\oem.ini；请关闭 WPS 后右键以管理员身份运行安装程序。详细原因：$($_.Exception.Message)"
  }
  Set-WpsAuthAddinEntryAllowed -Path (Join-Path $JsDir "authaddin.json") -Names $KnownNames -AddinName $AddinDisplayName -AddinPath (ConvertTo-FileUri -Path $Target)
  Clear-WpsJsAddinBlockHost -JsDir $JsDir
  Write-InstallState -Root $InstallDir -WpsInfo $WpsInfo -PluginsXml $PluginsXml
  Write-Host "已安装 publish 离线模式加载项目录：$Target"
  Write-InstallProgress -Percent 86 -Action "注册卸载入口" -Detail "正在写入开始菜单和控制面板卸载信息"
  New-UninstallShortcut -Root $InstallDir
  Register-UninstallEntry -Root $InstallDir -VersionInfo $VersionInfo

  Write-InstallProgress -Percent 100 -Action "安装完成" -Detail "请重启电脑后打开 WPS"
  Write-Host "WPS 文档朗读助手安装完成。请重启电脑后打开 WPS，在顶部查看“文档朗读”选项卡。"
  Write-Host "安装日志：$LogFile"
}
catch {
  Write-InstallProgress -Percent 100 -Action "安装失败" -Detail $_.Exception.Message
  throw
}
finally {
  Stop-Transcript | Out-Null
}
