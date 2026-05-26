param(
  [string]$Owner = "iwannawi",
  [string]$Repo = "wps-read-aloud-comate",
  [string]$Branch = "main",
  [string]$Tag = "",
  [switch]$PromptToken
)

$ErrorActionPreference = "Continue"
if (Get-Variable PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
  $PSNativeCommandUseErrorActionPreference = $false
}

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$LogDir = Join-Path $Root "build\logs"
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
$Log = Join-Path $LogDir "github-push.log"

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
  $Gcm = Get-Command git-credential-manager.exe -ErrorAction SilentlyContinue
  if (!$Gcm) {
    return ""
  } else {
    $GcmPath = $Gcm.Source
  }
  $InputText = "protocol=https`nhost=github.com`npath=$Owner/$Repo.git`n`n"
  $Cred = $InputText | & $GcmPath get
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
  $Gcm = Get-Command git-credential-manager.exe -ErrorAction SilentlyContinue
  if (!$Gcm) {
    return
  } else {
    $GcmPath = $Gcm.Source
  }
  $InputText = "protocol=https`nhost=github.com`npath=$Owner/$Repo.git`nusername=x-access-token`npassword=$Token`n`n"
  $InputText | & $GcmPath approve | Out-Null
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
  & curl.exe -sS --fail-with-body -L `
    -H "Authorization: Bearer $Token" `
    -H "Accept: application/vnd.github+json" `
    -H "User-Agent: Codex-WPS-Read-Aloud" `
    "https://api.github.com/user" | Out-Null
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
  return ((Test-GitHubToken $Token) -and (Test-GitHubGitAccess $Token))
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
    throw "GitHub token cannot access this repository through HTTPS Git."
  }
  Save-GitHubTokenToGcm $Token
  return $Token
}

function Invoke-Git($DisplayArgs, $ActualArgs, $Token) {
  Write-Log ("git " + ($DisplayArgs -join " "))
  $AuthBytes = [Text.Encoding]::ASCII.GetBytes("x-access-token:$Token")
  $AuthHeader = "Authorization: Basic " + [Convert]::ToBase64String($AuthBytes)
  $ActualArgsWithAuth = @("-c", "credential.helper=", "-c", "http.extraHeader=$AuthHeader") + $ActualArgs
  $AskPass = Join-Path $env:TEMP "wps-read-aloud-git-askpass.cmd"
  $OldAskPass = $env:GIT_ASKPASS
  $OldTerminalPrompt = $env:GIT_TERMINAL_PROMPT
  $OldUsername = $env:WPS_READ_ALOUD_GIT_USERNAME
  $OldToken = $env:WPS_READ_ALOUD_GIT_TOKEN
  try {
    @"
@echo off
echo %* | findstr /i "Username" >nul
if %errorlevel%==0 (
  echo %WPS_READ_ALOUD_GIT_USERNAME%
) else (
  echo %WPS_READ_ALOUD_GIT_TOKEN%
)
"@ | Set-Content -Encoding ASCII -Path $AskPass
    $env:GIT_ASKPASS = $AskPass
    $env:GIT_TERMINAL_PROMPT = "0"
    $env:WPS_READ_ALOUD_GIT_USERNAME = "x-access-token"
    $env:WPS_READ_ALOUD_GIT_TOKEN = $Token
    & git @ActualArgsWithAuth 2>&1 | Tee-Object -FilePath $Log -Append
    $ExitCode = $LASTEXITCODE
    Write-Log "exit code: $ExitCode"
    if ($ExitCode -ne 0) {
      throw "git failed with exit code $ExitCode"
    }
  } finally {
    if ($null -eq $OldAskPass) { Remove-Item Env:\GIT_ASKPASS -ErrorAction SilentlyContinue } else { $env:GIT_ASKPASS = $OldAskPass }
    if ($null -eq $OldTerminalPrompt) { Remove-Item Env:\GIT_TERMINAL_PROMPT -ErrorAction SilentlyContinue } else { $env:GIT_TERMINAL_PROMPT = $OldTerminalPrompt }
    if ($null -eq $OldUsername) { Remove-Item Env:\WPS_READ_ALOUD_GIT_USERNAME -ErrorAction SilentlyContinue } else { $env:WPS_READ_ALOUD_GIT_USERNAME = $OldUsername }
    if ($null -eq $OldToken) { Remove-Item Env:\WPS_READ_ALOUD_GIT_TOKEN -ErrorAction SilentlyContinue } else { $env:WPS_READ_ALOUD_GIT_TOKEN = $OldToken }
    Remove-Item -Force -LiteralPath $AskPass -ErrorAction SilentlyContinue
  }
}

if (Test-Path $Log) {
  Remove-Item -Force -LiteralPath $Log
}
Set-Location $Root

$Token = Get-GitHubToken
if ([string]::IsNullOrWhiteSpace($Token)) {
  throw "GitHub token is empty."
}

$CommonArgs = @(
  "-c", "safe.directory=$($Root.Path.Replace('\', '/'))",
  "-c", "http.sslBackend=openssl"
)

Invoke-Git @("push", "origin", $Branch) ($CommonArgs + @("push", "origin", $Branch)) $Token
if (![string]::IsNullOrWhiteSpace($Tag)) {
  Invoke-Git @("push", "origin", $Tag) ($CommonArgs + @("push", "origin", $Tag)) $Token
}

$Token = $null
Write-Log "Push completed."
Write-Host "Push completed."
