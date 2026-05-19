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

Unregister-ScheduledTask -TaskName "WPSReadAloudComate" -Confirm:$false
Remove-Item -LiteralPath $InstallDir -Recurse -Force
$JsDir = Join-Path $env:APPDATA "Kingsoft\wps\jsaddons"
Get-ChildItem -Path $JsDir -Directory -Filter "wps-read-aloud_*" | Remove-Item -Recurse -Force
Remove-WpsPluginEntry -Path (Join-Path $JsDir "publish.xml")
Remove-WpsPluginEntry -Path (Join-Path $JsDir "jsplugins.xml")
Write-Host "WPS 文档朗读助手已卸载。"
