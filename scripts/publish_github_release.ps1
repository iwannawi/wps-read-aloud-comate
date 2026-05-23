param(
  [string]$Owner = "iwannawi",
  [string]$Repo = "wps-read-aloud-comate",
  [string]$Version = "1.1.9",
  [string]$ReleaseDate = "20260523",
  [string]$Tag = "",
  [switch]$PromptToken
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Tag)) {
  $Tag = "v$Version-$ReleaseDate"
}

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$PlatformMatrix = Join-Path $Root "packaging\platforms.json"
$ReleaseNotes = Join-Path $Root "RELEASE_NOTES.md"
$Checksums = Join-Path $Root "CHECKSUMS.txt"
$Log = Join-Path $Root "dist\github-release-${Tag}.log"

function Write-Log($Text) {
  $Text | Tee-Object -FilePath $Log -Append
}

function ConvertFrom-SecureStringPlain($SecureString) {
  $Ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureString)
  try {
    return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($Ptr)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($Ptr)
  }
}

function Get-GitHubTokenFromGcm {
  $InputText = "protocol=https`nhost=github.com`npath=$Owner/$Repo.git`n`n"
  $Cred = $InputText | & git credential fill
  if ($LASTEXITCODE -ne 0 -or !$Cred) {
    return ""
  }
  foreach ($Line in $Cred) {
    if ($Line.StartsWith("password=")) {
      return $Line.Substring("password=".Length)
    }
  }
  return ""
}

function Save-GitHubTokenToGcm($Token) {
  if ([string]::IsNullOrWhiteSpace($Token)) {
    return
  }
  $InputText = "protocol=https`nhost=github.com`npath=$Owner/$Repo.git`nusername=x-access-token`npassword=$Token`n`n"
  $InputText | & git credential approve | Out-Null
}

function Get-GitHubTokenFromGh {
  $Gh = Get-Command gh.exe -ErrorAction SilentlyContinue
  if (!$Gh) {
    return ""
  }
  $Token = & $Gh.Source auth token 2>$null
  if ($LASTEXITCODE -ne 0 -or !$Token) {
    return ""
  }
  return (($Token | Out-String).Trim())
}

function Test-GitHubToken($Token) {
  if ([string]::IsNullOrWhiteSpace($Token)) {
    return $false
  }
  & curl.exe --ssl-no-revoke -sS --fail-with-body -L `
    -H "Authorization: Bearer $Token" `
    -H "Accept: application/vnd.github+json" `
    -H "User-Agent: Codex-WPS-Read-Aloud" `
    "https://api.github.com/user" | Out-Null
  return ($LASTEXITCODE -eq 0)
}

function Test-GitHubRepoApiAccess($Token) {
  if ([string]::IsNullOrWhiteSpace($Token)) {
    return $false
  }
  & curl.exe --ssl-no-revoke -sS --fail-with-body -L `
    -H "Authorization: Bearer $Token" `
    -H "Accept: application/vnd.github+json" `
    -H "User-Agent: Codex-WPS-Read-Aloud" `
    "https://api.github.com/repos/$Owner/$Repo" | Out-Null
  return ($LASTEXITCODE -eq 0)
}

function Test-GitHubGitAccess($Token) {
  if ([string]::IsNullOrWhiteSpace($Token)) {
    return $false
  }
  $AuthBytes = [Text.Encoding]::ASCII.GetBytes("x-access-token:$Token")
  $AuthHeader = "Authorization: Basic " + [Convert]::ToBase64String($AuthBytes)
  & git `
    -c "safe.directory=$($Root.Path.Replace('\', '/'))" `
    -c "http.sslBackend=openssl" `
    -c "http.extraHeader=$AuthHeader" `
    ls-remote "https://github.com/$Owner/$Repo.git" HEAD 2>$null | Out-Null
  return ($LASTEXITCODE -eq 0)
}

function Test-GitHubCredential($Token) {
  return ((Test-GitHubToken $Token) -and (Test-GitHubRepoApiAccess $Token) -and (Test-GitHubGitAccess $Token))
}

function Get-GitHubToken {
  if (!$PromptToken) {
    $Token = Get-GitHubTokenFromGh
    if (Test-GitHubCredential $Token) {
      Write-Log "Using GitHub token from gh auth."
      return $Token
    } elseif (![string]::IsNullOrWhiteSpace($Token)) {
      Write-Log "GitHub CLI token is present but cannot access this repository through HTTPS Git."
    }
    $Token = Get-GitHubTokenFromGcm
    if (Test-GitHubCredential $Token) {
      Write-Log "Using GitHub token from Git Credential Manager."
      return $Token
    } elseif (![string]::IsNullOrWhiteSpace($Token)) {
      Write-Log "Git Credential Manager token is present but cannot access this repository through HTTPS Git."
    }
  }
  Write-Log "No stored GitHub credential found; prompting for token."
  $Secure = Read-Host "Enter GitHub token" -AsSecureString
  $Token = ConvertFrom-SecureStringPlain $Secure
  if (!(Test-GitHubCredential $Token)) {
    throw "GitHub token cannot access this repository through GitHub API and HTTPS Git. Use a classic token with repo scope, or a fine-grained token with Contents read/write and Metadata read for this repository."
  }
  Save-GitHubTokenToGcm $Token
  return $Token
}

function Invoke-CurlJson($Arguments) {
  $Output = & curl.exe --ssl-no-revoke @Arguments 2>&1
  if ($LASTEXITCODE -ne 0) {
    if ($Output) {
      Write-Log ($Output | Out-String)
    }
    throw "curl failed with exit code $LASTEXITCODE"
  }
  return (($Output | Out-String) | ConvertFrom-Json)
}

foreach ($Path in @($PlatformMatrix, $ReleaseNotes, $Checksums)) {
  if (!(Test-Path $Path)) {
    throw "Missing required file: $Path"
  }
}
$Targets = @((Get-Content -Raw -Encoding UTF8 $PlatformMatrix | ConvertFrom-Json).targets)
$Artifacts = @()
foreach ($Target in $Targets) {
  $Artifact = Join-Path $Root ("dist\" + $Target.artifact)
  $ShaFile = "$Artifact.sha256"
  $Artifacts += $Artifact
  $Artifacts += $ShaFile
}
foreach ($Path in $Artifacts) {
  if (!(Test-Path $Path)) {
    throw "Missing required release artifact: $Path"
  }
}
$PythonCandidates = @(
  "C:\Users\zhangjingyao\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe",
  "python"
)
$Python = ""
foreach ($Candidate in $PythonCandidates) {
  if ($Candidate -eq "python") {
    $Cmd = Get-Command python -ErrorAction SilentlyContinue
    if ($Cmd) {
      $Python = $Cmd.Source
      break
    }
  } elseif (Test-Path $Candidate) {
    $Python = $Candidate
    break
  }
}
if ([string]::IsNullOrWhiteSpace($Python)) {
  throw "Python is required to verify the five release artifacts before publishing."
}
& $Python (Join-Path $Root "packaging\verify_release_artifacts.py")
if ($LASTEXITCODE -ne 0) {
  throw "Five-package release verification failed."
}
if (Test-Path $Log) {
  Remove-Item -Force -LiteralPath $Log
}

$Token = Get-GitHubToken
if ([string]::IsNullOrWhiteSpace($Token)) {
  throw "GitHub token is empty."
}

$Body = [System.IO.File]::ReadAllText($ReleaseNotes, [System.Text.Encoding]::UTF8)
$Body = $Body + "`n`n## SHA256`n`n````text`n" + [System.IO.File]::ReadAllText($Checksums, [System.Text.Encoding]::ASCII).Trim() + "`n````"
$ReleaseName = "$Repo $Version $ReleaseDate"

$Payload = @{
  tag_name = $Tag
  target_commitish = "main"
  name = $ReleaseName
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
  $GetArgs = @("-sS", "--fail-with-body", "-L") + $BaseHeaders + @(
    "https://api.github.com/repos/$Owner/$Repo/releases/tags/$Tag"
  )
  $Release = Invoke-CurlJson $GetArgs
}

$UploadBase = [regex]::Replace([string]$Release.upload_url, "\{.*\}$", "")
Write-Log ("Release URL: " + $Release.html_url)
Write-Log ("Upload API: " + $UploadBase)

$ExistingAssetsArgs = @("-sS", "--fail-with-body", "-L") + $BaseHeaders + @(
  "https://api.github.com/repos/$Owner/$Repo/releases/$($Release.id)/assets"
)
$ExistingAssets = @()
try {
  $ExistingAssets = @(Invoke-CurlJson $ExistingAssetsArgs)
} catch {
  Write-Log "Could not list existing assets; continuing with upload."
}

foreach ($Asset in $Artifacts) {
  $Name = [IO.Path]::GetFileName($Asset)
  $ContentType = "text/plain"
  if ($Name.EndsWith(".deb")) {
    $ContentType = "application/vnd.debian.binary-package"
  } elseif ($Name.EndsWith(".zip")) {
    $ContentType = "application/zip"
  } elseif ($Name.EndsWith(".exe")) {
    $ContentType = "application/vnd.microsoft.portable-executable"
  }
  foreach ($Existing in $ExistingAssets) {
    if ($Existing.name -eq $Name) {
      Write-Log "Deleting existing asset: $Name"
      $DeleteArgs = @("-sS", "--fail-with-body", "-L", "-X", "DELETE") + $BaseHeaders + @(
        "https://api.github.com/repos/$Owner/$Repo/releases/assets/$($Existing.id)"
      )
      & curl.exe --ssl-no-revoke @DeleteArgs | Out-Null
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
  & curl.exe --ssl-no-revoke @UploadArgs | Out-Null
  if ($LASTEXITCODE -ne 0) {
    throw "Asset upload failed: $Name"
  }
}

$Token = $null
Write-Log "GitHub Release published successfully."
Write-Host ("Published: " + $Release.html_url)
Write-Host ("Log file: " + $Log)
