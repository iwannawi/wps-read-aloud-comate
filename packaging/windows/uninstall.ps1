param(
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\WPS Read Aloud Comate",
  [switch]$Silent
)

$ErrorActionPreference = "SilentlyContinue"

function Show-Result {
  param(
    [string]$Title,
    [string]$Message,
    [int]$Icon = 64
  )
  if ($Silent) {
    return
  }
  try {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show($Message, $Title, "OK", $Icon) | Out-Null
  }
  catch {
    Write-Host $Message
  }
}

function Backup-ConfigFile {
  param([string]$Path)
  if (Test-Path $Path) {
    $Stamp = Get-Date -Format "yyyyMMddHHmmss"
    Copy-Item -LiteralPath $Path -Destination "$Path.bak.$Stamp" -Force
  }
}

function Remove-ProjectPluginEntries {
  param([string]$Content)
  $Pattern = 'wps-read-aloud|WPS Read Aloud|WPS 文档朗读助手|文档朗读助手|127\.0\.0\.1:19860|WPSReadAloudComate'
  $Content = [regex]::Replace($Content, "(?is)\s*<jspluginonline\b(?=[^>]*($Pattern))[^>]*/>", "")
  $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b(?=[^>]*($Pattern))[^>]*/>", "")
  $Content = [regex]::Replace($Content, "(?is)\s*<jsplugin\b(?=[^>]*($Pattern))[\s\S]*?</jsplugin>", "")
  return $Content
}

function Remove-WpsPluginEntry {
  param([string]$Path)
  if (!(Test-Path $Path)) {
    return
  }
  Backup-ConfigFile -Path $Path
  $Content = Get-Content -Raw -Path $Path -Encoding UTF8
  $Content = Remove-ProjectPluginEntries -Content $Content
  Set-Content -Path $Path -Value $Content -Encoding UTF8
}

function Remove-WpsAuthEntry {
  param([string]$Path)
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
    $RemoveKeys = @()
    foreach ($Property in $Data.wps.PSObject.Properties) {
      if ($Property.Name -eq "namelist") {
        continue
      }
      $Value = $Property.Value
      $Name = [string]$Value.name
      $PathValue = [string]$Value.path
      if ($Name -match 'wps-read-aloud|WPS 文档朗读助手|文档朗读助手' -or $PathValue -match 'wps-read-aloud|127\.0\.0\.1:19860|WPSReadAloudComate') {
        $RemoveKeys += $Property.Name
      }
    }
    foreach ($Key in $RemoveKeys) {
      $Data.wps.PSObject.Properties.Remove($Key)
      $Changed = $true
    }
    if ($Changed) {
      $Data | ConvertTo-Json -Depth 10 | Set-Content -Path $Path -Encoding UTF8
    }
  }
  catch {
  }
}

function Remove-WpsOemConfig {
  param([string]$Root)
  $StatePath = Join-Path $Root "install-state.json"
  if (!(Test-Path $StatePath)) {
    return
  }
  try {
    $State = Get-Content -Raw -Path $StatePath -Encoding UTF8 | ConvertFrom-Json
    $OemPath = [string]$State.wpsOemIni
    $Server = [string]$State.jsPluginsServer
    if ([string]::IsNullOrWhiteSpace($OemPath) -or !(Test-Path $OemPath)) {
      return
    }
    Backup-ConfigFile -Path $OemPath
    $Lines = New-Object System.Collections.Generic.List[string]
    foreach ($Line in (Get-Content -Path $OemPath -Encoding UTF8)) {
      if ($Line -match '^\s*JSPluginsServer\s*=' -and ($Line -match [regex]::Escape($Server) -or $Line -match 'Kingsoft[\\/]+wps[\\/]+jsaddons[\\/]+jsplugins\.xml')) {
        continue
      }
      $Lines.Add($Line)
    }
    Set-Content -Path $OemPath -Value ($Lines -join "`r`n") -Encoding UTF8
  }
  catch {
  }
}

function Stop-DaemonProcess {
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

function Remove-InstallDirAfterExit {
  param([string]$Root)
  if (!(Test-Path $Root)) {
    return
  }
  $PowerShell = Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"
  if (!(Test-Path $PowerShell)) {
    $PowerShell = "powershell.exe"
  }
  $Command = "Start-Sleep -Milliseconds 800; Remove-Item -LiteralPath '$Root' -Recurse -Force -ErrorAction SilentlyContinue"
  Start-Process -FilePath $PowerShell -WindowStyle Hidden -ArgumentList @("-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", $Command) | Out-Null
}

try {
  Stop-DaemonProcess -Root $InstallDir
  Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "WPSReadAloudComate" -ErrorAction SilentlyContinue
  try {
    Stop-ScheduledTask -TaskName "WPSReadAloudComate" -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName "WPSReadAloudComate" -Confirm:$false -ErrorAction SilentlyContinue
  }
  catch {
  }

  $JsDir = Join-Path $env:APPDATA "Kingsoft\wps\jsaddons"
  if (Test-Path $JsDir) {
    Get-ChildItem -Path $JsDir -Directory -Filter "wps-read-aloud_*" -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
    Get-ChildItem -Path $JsDir -Directory -Filter "文档朗读助手_*" -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
    Remove-WpsPluginEntry -Path (Join-Path $JsDir "publish.xml")
    Remove-WpsPluginEntry -Path (Join-Path $JsDir "jsplugins.xml")
    Remove-WpsAuthEntry -Path (Join-Path $JsDir "authaddin.json")
    Remove-Item -LiteralPath (Join-Path $JsDir "jsaddinblockhost.ini") -Force -ErrorAction SilentlyContinue
  }

  Remove-WpsOemConfig -Root $InstallDir
  Remove-Item -LiteralPath (Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\WPS 文档朗读助手") -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath (Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\WPS文档朗读助手") -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath (Join-Path $env:ProgramData "Microsoft\Windows\Start Menu\Programs\WPS 文档朗读助手") -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath (Join-Path $env:ProgramData "Microsoft\Windows\Start Menu\Programs\WPS文档朗读助手") -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\WPSReadAloudComate" -Recurse -Force -ErrorAction SilentlyContinue
  Set-Location $env:TEMP
  Remove-InstallDirAfterExit -Root $InstallDir
  Show-Result -Title "WPS 文档朗读助手" -Message "卸载完成。已停止本地朗读服务，并清理加载项配置、开始菜单入口和卸载注册表信息。"
}
catch {
  Show-Result -Title "WPS 文档朗读助手卸载失败" -Message ("卸载没有完成：" + $_.Exception.Message) -Icon 16
  exit 1
}
