param(
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\WPS Read Aloud Comate"
)

$ErrorActionPreference = "SilentlyContinue"

function Remove-WpsPluginEntry {
  param([string]$Path)
  if (!(Test-Path $Path)) {
    return
  }
  $Stamp = Get-Date -Format "yyyyMMddHHmmss"
  Copy-Item -LiteralPath $Path -Destination "$Path.bak.$Stamp" -Force
  $Content = Get-Content -Raw -Path $Path -Encoding UTF8
  $Content = [regex]::Replace($Content, '(?is)\s*<jspluginonline\b[^>]*name="wps-read-aloud"[^>]*/>', '')
  $Content = [regex]::Replace($Content, '(?is)\s*<jsplugin\b[^>]*name="wps-read-aloud"[\s\S]*?</jsplugin>', '')
  Set-Content -Path $Path -Value $Content -Encoding UTF8
}

function Stop-DaemonProcess {
  param([string]$Root)
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

Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "WPSReadAloudComate" -ErrorAction SilentlyContinue
Stop-DaemonProcess -Root $InstallDir
try {
  Stop-ScheduledTask -TaskName "WPSReadAloudComate" -ErrorAction SilentlyContinue
  Unregister-ScheduledTask -TaskName "WPSReadAloudComate" -Confirm:$false -ErrorAction SilentlyContinue
}
catch {
}
Remove-Item -LiteralPath $InstallDir -Recurse -Force
$JsDir = Join-Path $env:APPDATA "Kingsoft\wps\jsaddons"
Get-ChildItem -Path $JsDir -Directory -Filter "wps-read-aloud_*" | Remove-Item -Recurse -Force
Remove-WpsPluginEntry -Path (Join-Path $JsDir "publish.xml")
Remove-WpsPluginEntry -Path (Join-Path $JsDir "jsplugins.xml")
Write-Host "WPS 文档朗读助手已卸载。"
