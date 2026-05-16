param(
  [string]$Owner = "iwannawi",
  [string]$Repo = "wps-read-aloud",
  [string]$Tag = "v1.0.14-20260516"
)

$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$Deb = Join-Path $Root "dist\wps-read-aloud-zhangjingyao_1.0.14_arm64.deb"
$ShaFile = Join-Path $Root "dist\wps-read-aloud-zhangjingyao_1.0.14_arm64.deb.sha256"
$ReleaseNotes = Join-Path $Root "RELEASE_NOTES.md"
$Log = Join-Path $Root "dist\github-release-v1.0.14-20260516.log"

function Write-Log($Text) {
  $Text | Tee-Object -FilePath $Log -Append
}

function Invoke-CurlJson($Arguments) {
  $Output = & curl.exe @Arguments 2>&1
  if ($LASTEXITCODE -ne 0) {
    if ($Output) {
      Write-Log ($Output | Out-String)
    }
    throw "curl failed with exit code $LASTEXITCODE"
  }
  return (($Output | Out-String) | ConvertFrom-Json)
}

if (!(Test-Path $Deb)) {
  throw "Missing deb package: $Deb"
}
if (!(Test-Path $ShaFile)) {
  throw "Missing sha256 file: $ShaFile"
}
if (!(Test-Path $ReleaseNotes)) {
  throw "Missing release notes: $ReleaseNotes"
}

$Secure = Read-Host "Enter GitHub token" -AsSecureString
$Ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Secure)
try {
  $Token = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($Ptr)
} finally {
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($Ptr)
}
if ([string]::IsNullOrWhiteSpace($Token)) {
  throw "Token is empty."
}

if (Test-Path $Log) {
  Remove-Item -Force -LiteralPath $Log
}

$Body = [System.IO.File]::ReadAllText($ReleaseNotes, [System.Text.Encoding]::UTF8)
$Body = $Body + "`n`n## SHA256`n`n````text`n" + [System.IO.File]::ReadAllText($ShaFile, [System.Text.Encoding]::ASCII).Trim() + "`n````"

$Payload = @{
  tag_name = $Tag
  target_commitish = "main"
  name = $Tag
  body = $Body
  draft = $false
  prerelease = $false
} | ConvertTo-Json -Depth 5

$PayloadPath = Join-Path $env:TEMP "wps-read-aloud-release-payload.json"
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($PayloadPath, $Payload, $Utf8NoBom)

$BaseHeaders = @(
  "-H", "Authorization: Bearer $Token",
  "-H", "Accept: application/vnd.github+json",
  "-H", "X-GitHub-Api-Version: 2022-11-28",
  "-H", "User-Agent: Codex-WPS-Read-Aloud"
)

Write-Log "Creating or loading GitHub Release: $Tag"
$CreateArgs = @(
  "-sS", "--fail-with-body", "-L",
  "-X", "POST"
) + $BaseHeaders + @(
  "-H", "Content-Type: application/json",
  "--data-binary", "@$PayloadPath",
  "https://api.github.com/repos/$Owner/$Repo/releases"
)

try {
  $Release = Invoke-CurlJson $CreateArgs
} catch {
  Write-Log "Create failed; trying to load existing release by tag."
  $GetArgs = @(
    "-sS", "--fail-with-body", "-L"
  ) + $BaseHeaders + @(
    "https://api.github.com/repos/$Owner/$Repo/releases/tags/$Tag"
  )
  $Release = Invoke-CurlJson $GetArgs
}

$UploadBase = [regex]::Replace([string]$Release.upload_url, "\{.*\}$", "")
Write-Log ("Release URL: " + $Release.html_url)
Write-Log ("Upload API: " + $UploadBase)

$ExistingAssetsArgs = @(
  "-sS", "--fail-with-body", "-L"
) + $BaseHeaders + @(
  "https://api.github.com/repos/$Owner/$Repo/releases/$($Release.id)/assets"
)
$ExistingAssets = @()
try {
  $ExistingAssets = @(Invoke-CurlJson $ExistingAssetsArgs)
} catch {
  Write-Log "Could not list existing assets; continuing with upload."
}

foreach ($Asset in @($Deb, $ShaFile)) {
  $Name = [IO.Path]::GetFileName($Asset)
  $ContentType = "text/plain"
  if ($Name.EndsWith(".deb")) {
    $ContentType = "application/vnd.debian.binary-package"
  }
  foreach ($Existing in $ExistingAssets) {
    if ($Existing.name -eq $Name) {
      Write-Log "Deleting existing asset: $Name"
      $DeleteArgs = @(
        "-sS", "--fail-with-body", "-L",
        "-X", "DELETE"
      ) + $BaseHeaders + @(
        "https://api.github.com/repos/$Owner/$Repo/releases/assets/$($Existing.id)"
      )
      & curl.exe @DeleteArgs | Out-Null
      if ($LASTEXITCODE -ne 0) {
        throw "Asset delete failed: $Name"
      }
    }
  }
  $UploadUrl = $UploadBase + "?name=" + [uri]::EscapeDataString($Name)
  Write-Log "Uploading asset: $Name"
  Write-Log "Upload URL: $UploadUrl"
  $UploadArgs = @(
    "-sS", "--fail-with-body", "-L", "--globoff",
    "-X", "POST"
  ) + $BaseHeaders + @(
    "-H", "Content-Type: $ContentType",
    "--data-binary", "@$Asset",
    "--url", $UploadUrl
  )
  & curl.exe @UploadArgs | Out-Null
  if ($LASTEXITCODE -ne 0) {
    throw "Asset upload failed: $Name"
  }
}

Write-Log "GitHub Release published successfully."
Write-Host ""
Write-Host ("Published: " + $Release.html_url)
Write-Host ("Log file: " + $Log)
Read-Host "Press Enter to close"
