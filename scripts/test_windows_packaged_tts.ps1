param(
  [string]$Version = "",
  [string[]]$Sentences = @(),
  [double[]]$Rates = @(0.75, 1.0, 1.2, 1.5)
)

$ErrorActionPreference = "Stop"

function TextFromCodes([int[]]$Codes) {
  return -join ($Codes | ForEach-Object { [char]$_ })
}

if ($Sentences.Count -eq 0) {
  $toc = TextFromCodes @(0x76ee, 0x20, 0x20, 0x5f55)
  $background = TextFromCodes @(0x0058, 0x0043, 0x9879, 0x76ee, 0x80cc, 0x666f, 0x4e0e, 0x9700, 0x6c42)
  $format = TextFromCodes @(0x6587, 0x6863, 0x683c, 0x5f0f, 0x7edf, 0x4e00, 0x5ef6, 0x7eed)
  $product = TextFromCodes @(0x004f, 0x0066, 0x0066, 0x0069, 0x0063, 0x0065, 0x4ea7, 0x54c1, 0x65b9, 0x6848)
  $math = TextFromCodes @(0x8fdb, 0x5ea6, 0x003e, 0x003d, 0x0031, 0x0030, 0x0025, 0xff0c, 0x8bef, 0x5dee, 0x00b1, 0x0030, 0x002e, 0x0035, 0x0025)
  $Sentences = @(
    $toc,
    "1. $background`t1",
    "1.1. $format`t1",
    "WPS $product`t2",
    $math,
    $background
  )
}

function Get-FreeTcpPort {
  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Parse("127.0.0.1"), 0)
  $listener.Start()
  try {
    return [int]$listener.LocalEndpoint.Port
  } finally {
    $listener.Stop()
  }
}

function Expand-PayloadFromInstaller([string]$InstallerPath, [string]$Version) {
  if (!(Test-Path -LiteralPath $InstallerPath)) {
    throw "Windows installer does not exist: $InstallerPath"
  }
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  $marker = [System.Text.Encoding]::ASCII.GetBytes("WPS_READ_ALOUD_COMATE_PAYLOAD_ZIP_V1`n")
  $bytes = [System.IO.File]::ReadAllBytes($InstallerPath)
  $offset = -1
  for ($i = $bytes.Length - $marker.Length; $i -ge 0; $i -= 1) {
    $matched = $true
    for ($j = 0; $j -lt $marker.Length; $j += 1) {
      if ($bytes[$i + $j] -ne $marker[$j]) {
        $matched = $false
        break
      }
    }
    if ($matched) {
      $offset = $i + $marker.Length
      break
    }
  }
  if ($offset -lt 0) {
    throw "Installer payload marker was not found: $InstallerPath"
  }
  $tempRoot = Join-Path $env:TEMP ("wps-read-aloud-comate-test-" + $Version + "-" + [guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
  $zipPath = Join-Path $tempRoot "payload.zip"
  [System.IO.File]::WriteAllBytes($zipPath, $bytes[$offset..($bytes.Length - 1)])
  [System.IO.Compression.ZipFile]::ExtractToDirectory($zipPath, $tempRoot)
  return $tempRoot
}

if ([string]::IsNullOrWhiteSpace($Version)) {
  $platforms = Get-Content -Raw -Encoding UTF8 -LiteralPath "packaging\platforms.json" | ConvertFrom-Json
  $Version = [string]$platforms.version
}

$app = Join-Path (Get-Location) ("build\windows\wps-read-aloud-comate_{0}_windows_x86\app" -f $Version)
$extractedRoot = $null
$exe = Join-Path $app "daemon\wps-tts-daemon.exe"
if (!(Test-Path -LiteralPath $exe)) {
  $installer = Join-Path (Get-Location) ("dist\wps-read-aloud-comate_{0}_windows.exe" -f $Version)
  $extractedRoot = Expand-PayloadFromInstaller -InstallerPath $installer -Version $Version
  $app = Join-Path $extractedRoot "app"
  $exe = Join-Path $app "daemon\wps-tts-daemon.exe"
  if (!(Test-Path -LiteralPath $exe)) {
    throw "Windows packaged daemon does not exist in build directory or installer payload: $exe"
  }
}

$port = Get-FreeTcpPort
$baseUrl = "http://127.0.0.1:$port"
$cfg = Join-Path $app ("config-test-" + [guid]::NewGuid().ToString("N") + ".yaml")
$configLines = @(
  "listen: `"127.0.0.1:$port`"",
  "sherpa:",
  "  bin: `"engines/sherpa-onnx/sherpa-onnx-offline-tts.exe`"",
  "  num_threads: 4",
  "  target_sample_rate: 16000",
  "  vits_model: `"voices/sherpa/vits-zh-hf-fanchen-C/vits-zh-hf-fanchen-C.onnx`"",
  "  vits_lexicon: `"voices/sherpa/vits-zh-hf-fanchen-C/lexicon.txt`"",
  "  vits_tokens: `"voices/sherpa/vits-zh-hf-fanchen-C/tokens.txt`"",
  "  vits_rule_fsts: `"voices/sherpa/vits-zh-hf-fanchen-C/phone.fst,voices/sherpa/vits-zh-hf-fanchen-C/date.fst,voices/sherpa/vits-zh-hf-fanchen-C/number.fst,voices/sherpa/vits-zh-hf-fanchen-C/new_heteronym.fst`"",
  "  vits_speaker_id: 14"
)
Set-Content -LiteralPath $cfg -Encoding ASCII -Value $configLines

$process = Start-Process -FilePath $exe -WorkingDirectory $app -ArgumentList @("-config", $cfg) -WindowStyle Hidden -PassThru
try {
  $healthy = $false
  for ($i = 0; $i -lt 40; $i += 1) {
    try {
      $health = Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/health" -TimeoutSec 2
      if ($health.Content -match ('"version":"' + [regex]::Escape($Version) + '"')) {
        $healthy = $true
        break
      }
    } catch {}
    Start-Sleep -Milliseconds 500
  }
  if (!$healthy) {
    throw "Test service did not become healthy as $Version."
  }

  foreach ($rate in $Rates) {
    $body = @{
      sentences = @($Sentences | ForEach-Object { @{ text = $_ } })
      rate = $rate
      prefetch = 0
    } | ConvertTo-Json -Depth 5
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/read/start" -Method POST -ContentType "application/json" -Body $body -TimeoutSec 20 | Out-Null

    $reached = $false
    $last = ""
    for ($i = 0; $i -lt 80; $i += 1) {
      $last = (Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/read/status" -TimeoutSec 2).Content
      if ($last -match '"state":"playing"' -or $last -match '"state":"stopped"') {
        $reached = $true
        break
      }
      if ($last -match '"state":"error"') {
        throw "Read entered error state at rate ${rate}: $last"
      }
      Start-Sleep -Milliseconds 500
    }
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/read/stop" -Method POST -TimeoutSec 5 | Out-Null
    $postStopHealth = Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/health" -TimeoutSec 5
    if ($postStopHealth.StatusCode -lt 200 -or $postStopHealth.StatusCode -ge 500) {
      throw "Windows add-in host became unavailable after /read/stop at rate ${rate}."
    }
    if (!$reached) {
      throw "Read did not reach playable state at rate ${rate}: $last"
    }
  }

  Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/shutdown" -Method POST -TimeoutSec 5 | Out-Null
  $process.WaitForExit(8000) | Out-Null
  if (!$process.HasExited) {
    throw "Windows on-demand daemon did not exit after /shutdown."
  }

  [pscustomobject]@{
    Ok = $true
    Version = $Version
    Port = $port
    Sentences = $Sentences.Count
    Rates = $Rates
  } | ConvertTo-Json -Depth 4
} finally {
  try { Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/read/stop" -Method POST -TimeoutSec 2 | Out-Null } catch {}
  if (!$process.HasExited) {
    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
  }
  Remove-Item -LiteralPath $cfg -ErrorAction SilentlyContinue
  if ($extractedRoot -and (Test-Path -LiteralPath $extractedRoot)) {
    Remove-Item -LiteralPath $extractedRoot -Recurse -Force -ErrorAction SilentlyContinue
  }
}
