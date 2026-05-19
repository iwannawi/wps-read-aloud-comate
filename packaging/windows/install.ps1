param(
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\WPS Read Aloud Comate"
)

$ErrorActionPreference = "Stop"
$LogDir = Join-Path $env:LOCALAPPDATA "WPSReadAloudComate\Logs"
$LogFile = Join-Path $LogDir "install.log"
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
Start-Transcript -Path $LogFile -Append | Out-Null

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
    [string]$Entry
  )

  Backup-ConfigFile -Path $Path
  if (Test-Path $Path) {
    $Content = Get-Content -Raw -Path $Path -Encoding UTF8
    $Content = [regex]::Replace($Content, '(?is)\s*<jspluginonline\b[^>]*name="wps-read-aloud"[^>]*/>', '')
    $Content = [regex]::Replace($Content, '(?is)\s*<jsplugin\b[^>]*name="wps-read-aloud"[\s\S]*?</jsplugin>', '')
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
  throw "未找到符合版本要求的 WPS Office。Windows 平台最低要求 WPS Office 2019 或更高版本。`r`n已检测到：`r`n$Detected`r`n请升级或安装 WPS Office 后再运行本安装包。"
}

try {
  $Source = Join-Path $PSScriptRoot "app"
  if (!(Test-Path $Source)) {
    throw "安装包不完整：未找到 app 目录。"
  }
  $VersionInfo = Get-Content -Raw -Path (Join-Path $Source "version.json") | ConvertFrom-Json
  $PackageArch = $VersionInfo.architecture
  if ($PackageArch -eq "386") {
    $PackageArch = "x86"
  }
  $WpsInfo = Test-WpsRequirement -Installations (Find-WpsInstallations) -PackageArch $PackageArch
  Write-Host "检测到 WPS：$($WpsInfo.Path)"
  Write-Host "WPS 位数：$($WpsInfo.Architecture)；WPS 版本：$($WpsInfo.VersionText)"
  Write-Host "本安装包使用独立本地朗读服务，不注入 WPS 进程，可同时支持 32 位和 64 位 WPS。"

  $TaskName = "WPSReadAloudComate"
  Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  Copy-Item -Path (Join-Path $Source "*") -Destination $InstallDir -Recurse -Force

  $Daemon = Join-Path $InstallDir "daemon\wps-tts-daemon.exe"
  if (!(Test-Path $Daemon)) {
    throw "安装包不完整：未找到 wps-tts-daemon.exe。"
  }

  $Action = New-ScheduledTaskAction -Execute $Daemon -Argument "-config `"$InstallDir\config.yaml`"" -WorkingDirectory $InstallDir
  $Trigger = New-ScheduledTaskTrigger -AtLogOn
  $Principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited
  Register-ScheduledTask -TaskName $TaskName -Action $Action -Trigger $Trigger -Principal $Principal -Force | Out-Null
  Start-ScheduledTask -TaskName $TaskName

  $JsDir = Join-Path $env:APPDATA "Kingsoft\wps\jsaddons"
  New-Item -ItemType Directory -Force -Path $JsDir | Out-Null
  $AddinVersion = $VersionInfo.version
  $Target = Join-Path $JsDir "wps-read-aloud_$AddinVersion"
  New-Item -ItemType Directory -Force -Path $Target | Out-Null
  Copy-Item -Path (Join-Path $InstallDir "addin\*") -Destination $Target -Recurse -Force

  $Index = (Join-Path $Target "index.html").Replace("\", "/")
  $Ribbon = (Join-Path $Target "ribbon.xml").Replace("\", "/")
  $FileUrl = "file:///$Index"
  $LocalUrl = "http://127.0.0.1:19860/addin/index.html"
  $PublishXml = Join-Path $JsDir "publish.xml"
  $PluginsXml = Join-Path $JsDir "jsplugins.xml"
  $OnlineEntry = "<jspluginonline name=`"wps-read-aloud`" type=`"wps`" enable=`"enable_dev`" install=`"$LocalUrl`" url=`"$LocalUrl`" debug=`"`"/>"
  $LocalEntry = @"
<jsplugin name="wps-read-aloud" type="wps" url="$FileUrl" version="$AddinVersion" desc="WPS 文档朗读助手. Developer: Zhang Jingyao.">
    <ribbon file="$Ribbon"/>
  </jsplugin>
"@
  Set-WpsPluginEntry -Path $PublishXml -Entry $OnlineEntry
  Set-WpsPluginEntry -Path $PluginsXml -Entry $LocalEntry

  Write-Host "WPS 文档朗读助手安装完成。若 WPS 已打开，请重启 WPS。"
  Write-Host "安装日志：$LogFile"
}
finally {
  Stop-Transcript | Out-Null
}
