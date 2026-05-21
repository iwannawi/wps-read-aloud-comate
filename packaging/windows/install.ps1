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

function Remove-ProjectPluginEntries {
  param([string]$Content)
  $Pattern = 'wps-read-aloud|WPS Read Aloud|WPS 文档朗读助手|文档朗读助手|127\.0\.0\.1:19860'
  $Content = [regex]::Replace($Content, "(?is)\s*<jspluginonline\b(?=[^>]*($Pattern))[^>]*/>", "")
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
    $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b[^>]*name=`"$Escaped`"[\s\S]*?</jsplugin>", "")
  }
  Set-Content -Path $Path -Value $Content -Encoding UTF8
}

function Remove-WpsAuthAddinEntry {
  param(
    [string]$Path,
    [string]$Name
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
    $RemoveKeys = @()
    foreach ($Property in $Data.wps.PSObject.Properties) {
      if ($Property.Name -eq "namelist") {
        continue
      }
      if ($Property.Value.name -eq $Name) {
        $RemoveKeys += $Property.Name
      }
    }
    foreach ($Key in $RemoveKeys) {
      $Data.wps.PSObject.Properties.Remove($Key)
    }
    if ($RemoveKeys.Count -gt 0 -and $Data.wps.PSObject.Properties.Name -contains "namelist") {
      $NameList = [string]$Data.wps.namelist
      foreach ($Key in $RemoveKeys) {
        $NameList = $NameList.Replace($Key, "")
      }
      $Data.wps.namelist = $NameList.Trim(" ,;")
    }
    $Data | ConvertTo-Json -Depth 10 | Set-Content -Path $Path -Encoding UTF8
  }
  catch {
    Write-Host "旧版 WPS 加载项授权缓存清理失败，已跳过：$($_.Exception.Message)"
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

function Register-DaemonStartup {
  param([string]$Launcher)

  $RunKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
  New-Item -Path $RunKey -Force | Out-Null
  $PowerShell = Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"
  if (!(Test-Path $PowerShell)) {
    $PowerShell = "powershell.exe"
  }
  $Command = "`"$PowerShell`" -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$Launcher`""
  Set-ItemProperty -Path $RunKey -Name "WPSReadAloudComate" -Value $Command
  return $PowerShell
}

function Test-LocalServiceHealthy {
  try {
    $Response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:19860/health" -TimeoutSec 2
    return ($Response.StatusCode -ge 200 -and $Response.StatusCode -lt 500)
  }
  catch {
    return $false
  }
}

function Wait-LocalServiceHealthy {
  param([int]$TimeoutSeconds = 15)

  $Deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $Deadline) {
    if (Test-LocalServiceHealthy) {
      return $true
    }
    Start-Sleep -Milliseconds 500
  }
  return $false
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
  $TaskName = "WPSReadAloudComate"
  try {
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
  }
  catch {
    Write-Host "旧版计划任务清理被系统拒绝，已跳过；新版安装使用当前用户 Run 自启动。"
  }
  Stop-InstalledDaemonProcess -Root $InstallDir
  Write-InstallProgress -Percent 35 -Action "复制程序文件" -Detail "安装路径：$InstallDir"
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  Copy-Item -Path (Join-Path $Source "*") -Destination $InstallDir -Recurse -Force

  $Daemon = Join-Path $InstallDir "daemon\wps-tts-daemon.exe"
  if (!(Test-Path $Daemon)) {
    throw "安装包不完整：未找到 wps-tts-daemon.exe。"
  }

  $Launcher = New-DaemonLauncher -Root $InstallDir -Daemon $Daemon -Config (Join-Path $InstallDir "config.yaml")
  Write-InstallProgress -Percent 50 -Action "注册启动项" -Detail "正在写入当前用户自启动配置"
  Register-DaemonStartup -Launcher $Launcher | Out-Null
  Write-InstallProgress -Percent 58 -Action "启动朗读服务" -Detail "正在启动本机语音合成服务"
  Start-Process -FilePath $Daemon -ArgumentList @("-config", (Join-Path $InstallDir "config.yaml")) -WorkingDirectory $InstallDir -WindowStyle Hidden
  Write-Host "已注册当前用户登录自启动：HKCU\Software\Microsoft\Windows\CurrentVersion\Run\WPSReadAloudComate"
  if (!(Wait-LocalServiceHealthy -TimeoutSeconds 60)) {
    throw "本地朗读服务未能启动，请查看安装目录中的 start-daemon.ps1 和安装日志。"
  }
  Write-Host "本地朗读服务已启动：http://127.0.0.1:19860/health"

  Write-InstallProgress -Percent 70 -Action "注册 WPS 加载项" -Detail "正在写入 WPS 加载项配置"
  $JsDir = Join-Path $env:APPDATA "Kingsoft\wps\jsaddons"
  New-Item -ItemType Directory -Force -Path $JsDir | Out-Null
  $AddinVersion = $VersionInfo.version
  $Target = Join-Path $JsDir "wps-read-aloud_$AddinVersion"
  New-Item -ItemType Directory -Force -Path $Target | Out-Null
  Copy-Item -Path (Join-Path $InstallDir "addin\*") -Destination $Target -Recurse -Force

  $LocalUrl = "http://127.0.0.1:19860/addin/"
  $PublishXml = Join-Path $JsDir "publish.xml"
  $PluginsXml = Join-Path $JsDir "jsplugins.xml"
  $KnownNames = @($AddinInternalName, $AddinDisplayName, "WPS 文档朗读助手", "wps-read-aloud-comate", "wps-read-aloud-xc", "wps-read-aloud-zhangjingyao")
  $OnlineEntry = @"
<jspluginonline name="$AddinDisplayName" type="wps" enable="enable" install="$LocalUrl" url="$LocalUrl" desc="$AddinDescription"/>
"@
  Set-WpsPluginEntry -Path $PublishXml -Entry $OnlineEntry -Names $KnownNames
  Remove-WpsPluginEntry -Path $PluginsXml -Names $KnownNames
  Remove-WpsAuthAddinEntry -Path (Join-Path $JsDir "authaddin.json") -Name $AddinInternalName
  Remove-WpsAuthAddinEntry -Path (Join-Path $JsDir "authaddin.json") -Name $AddinDisplayName
  Clear-WpsJsAddinBlockHost -JsDir $JsDir

  Write-InstallProgress -Percent 100 -Action "安装完成" -Detail "请彻底退出并重新打开 WPS"
  Write-Host "WPS 文档朗读助手安装完成。若 WPS 已打开，请重启 WPS。"
  Write-Host "安装日志：$LogFile"
}
catch {
  Write-InstallProgress -Percent 100 -Action "安装失败" -Detail $_.Exception.Message
  throw
}
finally {
  Stop-Transcript | Out-Null
}
