param(
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\WPS Read Aloud Comate"
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

[System.Windows.Forms.Application]::EnableVisualStyles()
[System.Windows.Forms.Application]::SetCompatibleTextRenderingDefault($false)

$script:installDone = $false
$script:exitCode = $null
$script:lastLineCount = 0
$script:process = $null

$progressFile = Join-Path $env:TEMP ("wps-read-aloud-comate-progress-" + [guid]::NewGuid().ToString("N") + ".log")
$installScript = Join-Path $PSScriptRoot "install.ps1"
$logFile = Join-Path $env:LOCALAPPDATA "WPSReadAloudComate\Logs\install.log"
$assetDir = Join-Path $PSScriptRoot "installer-assets"
$iconPath = Join-Path $assetDir "app.ico"
$headerPath = Join-Path $assetDir "installer-header.png"

function Get-PowerShellPath {
  $candidates = @(
    (Join-Path $env:WINDIR "Sysnative\WindowsPowerShell\v1.0\powershell.exe"),
    (Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"),
    "powershell.exe"
  )
  foreach ($candidate in $candidates) {
    if ($candidate -eq "powershell.exe" -or (Test-Path $candidate)) {
      return $candidate
    }
  }
  return "powershell.exe"
}

function Read-LogTail {
  if (!(Test-Path $logFile)) {
    return "未生成安装日志。"
  }
  try {
    return ((Get-Content -Path $logFile -Tail 18 -Encoding UTF8) -join "`r`n")
  }
  catch {
    return "安装日志读取失败：$($_.Exception.Message)"
  }
}

function Update-ProgressFromFile {
  if (!(Test-Path $progressFile)) {
    return
  }
  $lines = @(Get-Content -Path $progressFile -Encoding UTF8 -ErrorAction SilentlyContinue)
  if ($lines.Count -eq 0) {
    return
  }
  if ($script:lastLineCount -ge $lines.Count) {
    return
  }
  $newLines = $lines[$script:lastLineCount..($lines.Count - 1)]
  $script:lastLineCount = $lines.Count
  foreach ($line in $newLines) {
    if ([string]::IsNullOrWhiteSpace($line)) {
      continue
    }
    try {
      $item = $line | ConvertFrom-Json
      $progressBar.Value = [Math]::Max(0, [Math]::Min(100, [int]$item.percent))
      $actionLabel.Text = [string]$item.action
      $detailLabel.Text = [string]$item.detail
      if ($item.detail) {
        $detailBox.AppendText($item.time + "  " + $item.action + "  " + $item.detail + "`r`n")
      }
    }
    catch {
    }
  }
}

$form = New-Object System.Windows.Forms.Form
$form.Text = "WPS 文档朗读助手 安装程序"
$form.StartPosition = "CenterScreen"
$form.FormBorderStyle = "FixedDialog"
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$form.ClientSize = New-Object System.Drawing.Size(900, 760)
$form.AutoScaleMode = [System.Windows.Forms.AutoScaleMode]::Dpi
$form.Font = New-Object System.Drawing.Font("Microsoft YaHei UI", 10, [System.Drawing.FontStyle]::Regular, [System.Drawing.GraphicsUnit]::Point)
$form.BackColor = [System.Drawing.Color]::FromArgb(248, 250, 253)
if (Test-Path $iconPath) {
  try {
    $form.Icon = New-Object System.Drawing.Icon($iconPath)
  }
  catch {
  }
}

$header = New-Object System.Windows.Forms.Panel
$header.Location = New-Object System.Drawing.Point(0, 0)
$header.Size = New-Object System.Drawing.Size(900, 450)
$header.BackColor = [System.Drawing.Color]::White
$header.Add_Paint({
  param($sender, $e)
  $e.Graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
  $rect = $sender.ClientRectangle
  $brush = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
    $rect,
    [System.Drawing.Color]::FromArgb(246, 250, 255),
    [System.Drawing.Color]::FromArgb(224, 237, 255),
    [System.Drawing.Drawing2D.LinearGradientMode]::Horizontal
  )
  $e.Graphics.FillRectangle($brush, $rect)
  $brush.Dispose()

  $accentBrush = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(42, 37, 110, 235))
  $e.Graphics.FillEllipse($accentBrush, 556, -80, 230, 230)
  $accentBrush.Dispose()
  $accentBrush = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(24, 37, 110, 235))
  $e.Graphics.FillEllipse($accentBrush, 612, 74, 88, 88)
  $accentBrush.Dispose()
})
$form.Controls.Add($header)

$headerImage = New-Object System.Windows.Forms.PictureBox
$headerImage.Location = New-Object System.Drawing.Point(0, 0)
$headerImage.Size = New-Object System.Drawing.Size(900, 450)
$headerImage.SizeMode = "StretchImage"
$headerImage.BackColor = [System.Drawing.Color]::White
if (Test-Path $headerPath) {
  try {
    $headerImage.Image = [System.Drawing.Image]::FromFile($headerPath)
  }
  catch {
  }
}
$header.Controls.Add($headerImage)

$pathTitle = New-Object System.Windows.Forms.Label
$pathTitle.Text = "安装路径"
$pathTitle.AutoSize = $true
$pathTitle.ForeColor = [System.Drawing.Color]::FromArgb(36, 48, 64)
$pathTitle.Location = New-Object System.Drawing.Point(56, 474)
$pathTitle.UseCompatibleTextRendering = $false
$form.Controls.Add($pathTitle)

$pathBox = New-Object System.Windows.Forms.TextBox
$pathBox.Text = $InstallDir
$pathBox.ReadOnly = $true
$pathBox.BorderStyle = "FixedSingle"
$pathBox.Location = New-Object System.Drawing.Point(138, 469)
$pathBox.Size = New-Object System.Drawing.Size(706, 28)
$pathBox.ForeColor = [System.Drawing.Color]::FromArgb(20, 28, 42)
$form.Controls.Add($pathBox)

$progressBar = New-Object System.Windows.Forms.ProgressBar
$progressBar.Location = New-Object System.Drawing.Point(56, 522)
$progressBar.Size = New-Object System.Drawing.Size(788, 24)
$progressBar.Minimum = 0
$progressBar.Maximum = 100
$form.Controls.Add($progressBar)

$actionLabel = New-Object System.Windows.Forms.Label
$actionLabel.Text = "准备开始安装"
$actionLabel.Font = New-Object System.Drawing.Font("Microsoft YaHei UI", 10, [System.Drawing.FontStyle]::Bold)
$actionLabel.AutoSize = $true
$actionLabel.ForeColor = [System.Drawing.Color]::FromArgb(31, 41, 55)
$actionLabel.Location = New-Object System.Drawing.Point(56, 570)
$actionLabel.UseCompatibleTextRendering = $false
$form.Controls.Add($actionLabel)

$detailLabel = New-Object System.Windows.Forms.Label
$detailLabel.Text = "请稍候。"
$detailLabel.AutoSize = $true
$detailLabel.ForeColor = [System.Drawing.Color]::FromArgb(52, 64, 84)
$detailLabel.Location = New-Object System.Drawing.Point(56, 600)
$detailLabel.UseCompatibleTextRendering = $false
$form.Controls.Add($detailLabel)

$detailBox = New-Object System.Windows.Forms.TextBox
$detailBox.Multiline = $true
$detailBox.ReadOnly = $true
$detailBox.ScrollBars = "Vertical"
$detailBox.BorderStyle = "FixedSingle"
$detailBox.BackColor = [System.Drawing.Color]::White
$detailBox.ForeColor = [System.Drawing.Color]::FromArgb(20, 28, 42)
$detailBox.Font = New-Object System.Drawing.Font("Microsoft YaHei UI", 9.5)
$detailBox.Location = New-Object System.Drawing.Point(56, 632)
$detailBox.Size = New-Object System.Drawing.Size(788, 74)
$form.Controls.Add($detailBox)

$closeButton = New-Object System.Windows.Forms.Button
$closeButton.Text = "安装中"
$closeButton.Enabled = $false
$closeButton.Location = New-Object System.Drawing.Point(400, 720)
$closeButton.Size = New-Object System.Drawing.Size(100, 34)
$closeButton.Font = New-Object System.Drawing.Font("Microsoft YaHei UI", 10)
$closeButton.Add_Click({ $form.Close() })
$form.Controls.Add($closeButton)

$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 400
$timer.Add_Tick({
  Update-ProgressFromFile
  if ($script:process -and $script:process.HasExited -and !$script:installDone) {
    $script:installDone = $true
    $script:exitCode = $script:process.ExitCode
    $timer.Stop()
    Update-ProgressFromFile
    if ($script:exitCode -eq 0) {
      $progressBar.Value = 100
      $actionLabel.Text = "安装完成"
      $detailLabel.Text = "请彻底退出并重新打开 WPS，然后在顶部查看文档朗读选项卡。"
      $detailBox.AppendText("安装完成。建议重新打开 WPS 后使用。`r`n")
      $closeButton.Text = "完成"
      $closeButton.Enabled = $true
    }
    else {
      $actionLabel.Text = "安装失败"
      $detailLabel.Text = "安装没有完成，请根据下方原因处理后重新运行安装程序。"
      $detailBox.AppendText("安装失败，退出代码：" + $script:exitCode + "`r`n")
      $detailBox.AppendText((Read-LogTail) + "`r`n")
      $closeButton.Text = "关闭"
      $closeButton.Enabled = $true
    }
  }
})

$form.Add_Shown({
  try {
    if (!(Test-Path $installScript)) {
      throw "安装包不完整：未找到 install.ps1。"
    }
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = Get-PowerShellPath
    $psi.Arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$installScript`" -InstallDir `"$InstallDir`" -ProgressFile `"$progressFile`""
    $psi.WorkingDirectory = $PSScriptRoot
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $script:process = [System.Diagnostics.Process]::Start($psi)
    $timer.Start()
  }
  catch {
    $actionLabel.Text = "安装失败"
    $detailLabel.Text = $_.Exception.Message
    $detailBox.AppendText($_.Exception.Message + "`r`n")
    $closeButton.Text = "关闭"
    $closeButton.Enabled = $true
  }
})

[void]$form.ShowDialog()
if ($script:exitCode -and $script:exitCode -ne 0) {
  exit $script:exitCode
}
exit 0
