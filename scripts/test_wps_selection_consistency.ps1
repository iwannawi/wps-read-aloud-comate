param(
  [string]$DocumentPath = ("D:\Dev Projects\" + [string][char]0x7ef4 + [string][char]0x62a4 + [string][char]0x624b + [string][char]0x518c + ".docx"),
  [int]$MinPages = 0,
  [int]$MaxChecks = 0,
  [int[]]$ScenarioPages = @(2, 5, 10),
  [switch]$KeepWpsOpen,
  [switch]$SummaryOnly,
  [switch]$PreferWordAutomation
)

$ErrorActionPreference = "Stop"
$WdActiveEndPageNumber = 3
$SentenceEndPattern = '[\u3002\uff01\uff1f\uff1b\uff1a]+|[\r\n]+'
$SemanticSentenceEndPattern = '[\u3002\uff01\uff1f\uff1b\uff1a]+'
$MaxSentenceLength = 1000
$WdGoToPage = 1
$WdGoToAbsolute = 1

function Normalize-Text([string]$Text) {
  if ($null -eq $Text) { return "" }
  return ((($Text -replace "`r`n", "`n" -replace "`r", "`n") -replace '[\x00-\x08\x0B\x0C\x0E-\x1F]', '') -replace '[\uFFFC\uFFFD]', '').Trim()
}

function Get-SearchTexts([string]$Text) {
  $values = New-Object System.Collections.Generic.List[string]
  foreach ($candidate in @($Text, (Normalize-Text $Text))) {
    $value = [string]$candidate
    if ([string]::IsNullOrWhiteSpace($value)) { continue }
    if (!$values.Contains($value.Trim())) {
      $values.Add($value.Trim())
    }
  }
  return $values
}

function New-Segment([string]$Raw, [int]$Base, [int]$Start, [int]$End, [int]$ScopeStart = 0, [int]$ScopeEnd = 0) {
  if ($End -le $Start) { return $null }
  $text = $Raw.Substring($Start, $End - $Start)
  $trimmed = $text.Trim()
  if ([string]::IsNullOrWhiteSpace($trimmed)) { return $null }

  $leadingMatch = [regex]::Match($text, '\S')
  $leading = if ($leadingMatch.Success) { $leadingMatch.Index } else { 0 }
  $trailing = $text.Length - $text.TrimEnd().Length
  $localStart = $Start + $leading
  $localEnd = $End - $trailing
  $segmentText = if ($trimmed.Length -gt $MaxSentenceLength) { $trimmed.Substring(0, $MaxSentenceLength) } else { $trimmed }
  $segmentEnd = $Base + [Math]::Min($localEnd, $localStart + $MaxSentenceLength)

  [pscustomobject]@{
    Text       = $segmentText
    Start      = $Base + $localStart
    End        = $segmentEnd
    ScopeStart = if ($ScopeStart -gt 0) { $ScopeStart } else { $Base }
    ScopeEnd   = if ($ScopeEnd -gt 0) { $ScopeEnd } else { $Base + $Raw.Length }
  }
}

function Split-ReadSegments([string]$Raw, [int]$Base, [int]$RangeEnd = 0) {
  $segments = New-Object System.Collections.Generic.List[object]
  $start = 0
  $matched = $false
  foreach ($match in [regex]::Matches($Raw, $SentenceEndPattern)) {
    $matched = $true
    $end = $match.Index + $match.Length
    $segment = New-Segment -Raw $Raw -Base $Base -Start $start -End $end -ScopeStart $Base -ScopeEnd $RangeEnd
    if ($null -ne $segment -and $RangeEnd -gt 0 -and $segment.End -gt $RangeEnd) { $segment.End = $RangeEnd }
    if ($null -ne $segment) { $segments.Add($segment) }
    $start = $end
  }
  if (!$matched -and ![string]::IsNullOrWhiteSpace($Raw)) {
    $trimmed = $Raw.Trim()
    $segments.Add([pscustomobject]@{
      Text       = if ($trimmed.Length -gt $MaxSentenceLength) { $trimmed.Substring(0, $MaxSentenceLength) } else { $trimmed }
      Start      = $Base
      End        = if ($RangeEnd -gt 0) { $RangeEnd } else { $Base + $Raw.Length }
      ScopeStart = $Base
      ScopeEnd   = if ($RangeEnd -gt 0) { $RangeEnd } else { $Base + $Raw.Length }
    })
    return $segments
  }
  $tail = New-Segment -Raw $Raw -Base $Base -Start $start -End $Raw.Length -ScopeStart $Base -ScopeEnd $RangeEnd
  if ($null -ne $tail -and $RangeEnd -gt 0 -and $tail.End -gt $RangeEnd) { $tail.End = $RangeEnd }
  if ($null -ne $tail) { $segments.Add($tail) }
  return $segments
}

function Get-ParagraphSegments($Doc) {
  $segments = New-Object System.Collections.Generic.List[object]
  $count = [int]$Doc.Paragraphs.Count
  for ($i = 1; $i -le $count; $i += 1) {
    $range = $Doc.Paragraphs.Item($i).Range
    $text = [string]$range.Text
    if ([string]::IsNullOrWhiteSpace((Normalize-Text $text))) { continue }
    if ($text -notmatch $SemanticSentenceEndPattern) {
      $trimmed = $text.Trim()
      $segments.Add([pscustomobject]@{
        Text  = if ($trimmed.Length -gt $MaxSentenceLength) { $trimmed.Substring(0, $MaxSentenceLength) } else { $trimmed }
        Start = [int]$range.Start
        End   = [int]$range.End
        ScopeStart = [int]$range.Start
        ScopeEnd   = [int]$range.End
      })
    } else {
      $parts = Split-ReadSegments -Raw $text -Base ([int]$range.Start) -RangeEnd ([int]$range.End)
      foreach ($part in $parts) { $segments.Add($part) }
    }
  }
  return $segments
}

function Get-PageStart($Doc, $App, [int]$Page) {
  if ($Page -le 1) { return [int]$Doc.Content.Start }
  $attempts = @(
    { param($d, $a, $p) $d.GoTo($WdGoToPage, $WdGoToAbsolute, $p) },
    { param($d, $a, $p) $d.Range(0, 0).GoTo($WdGoToPage, $WdGoToAbsolute, $p) },
    { param($d, $a, $p) $a.Selection.GoTo($WdGoToPage, $WdGoToAbsolute, $p) }
  )
  foreach ($attempt in $attempts) {
    try {
      $range = & $attempt $Doc $App $Page
      if ($null -ne $range -and $null -ne $range.Start) {
        return [int]$range.Start
      }
    } catch {}
  }
  return $null
}

function Get-PageEnd($Doc, $App, [int]$Page) {
  $docEnd = [int]$Doc.Content.End
  $next = Get-PageStart -Doc $Doc -App $App -Page ($Page + 1)
  if ($null -ne $next -and $next -gt 0) {
    return [Math]::Min($docEnd, [int]$next - 1)
  }
  return $docEnd
}

function Select-ScenarioSegments($Segments, [int]$Start, [int]$End = 0, [int]$Limit = 0) {
  $selected = New-Object System.Collections.Generic.List[object]
  foreach ($segment in $Segments) {
    if ([int]$segment.Start -lt $Start) { continue }
    if ($End -gt 0 -and [int]$segment.Start -ge $End) { break }
    $selected.Add($segment)
    if ($Limit -gt 0 -and $selected.Count -ge $Limit) { break }
  }
  return $selected
}

function Test-Segments($Doc, $App, $Segments, [string]$Name, [int]$MinPagesRequired, [int]$Limit = 0) {
  $results = New-Object System.Collections.Generic.List[object]
  $pages = @{}
  foreach ($segment in $Segments) {
    if ($Limit -gt 0 -and $results.Count -ge $Limit -and $pages.Count -ge $MinPagesRequired) { break }
    $selection = Select-SegmentRange -Doc $Doc -App $App -Segment $segment
    $expectedPage = [int]$selection.ExpectedPage

    $selectedText = Normalize-Text ([string]$App.Selection.Range.Text)
    $expectedText = Normalize-Text ([string]$segment.Text)
    $selectedPage = 0
    try { $selectedPage = [int]$App.Selection.Information($WdActiveEndPageNumber) } catch {}
    if ($selectedPage -gt 0) { $pages["$selectedPage"] = $true }

    $textOk = $selectedText -eq $expectedText
    $pageOk = ($expectedPage -eq 0 -or $selectedPage -eq 0 -or $expectedPage -eq $selectedPage)
    $results.Add([pscustomobject]@{
      Scenario     = $Name
      Index        = $results.Count + 1
      ExpectedPage = $expectedPage
      SelectedPage = $selectedPage
      Start        = [int]$segment.Start
      End          = [int]$segment.End
      TextOk       = $textOk
      PageOk       = $pageOk
      Fallback     = [bool]$selection.Fallback
      ExpectedText = if ($expectedText.Length -gt 80) { $expectedText.Substring(0, 80) } else { $expectedText }
      SelectedText = if ($selectedText.Length -gt 80) { $selectedText.Substring(0, 80) } else { $selectedText }
    })

    if (!$textOk -or !$pageOk) { break }
  }

  $failed = @($results | Where-Object { -not $_.TextOk -or -not $_.PageOk })
  $firstFailed = $null
  if ($failed.Count -gt 0) {
    $firstFailed = $failed[0]
  }
  return [pscustomobject]@{
    Name            = $Name
    Ok              = ($failed.Count -eq 0 -and $pages.Count -ge $MinPagesRequired)
    CheckedSegments = $results.Count
    CoveredPages    = $pages.Count
    MinPages        = $MinPagesRequired
    FailedCount     = $failed.Count
    FirstFailure    = $firstFailed
    Results         = $results
  }
}

function Select-SegmentRange($Doc, $App, $Segment) {
  $range = $Doc.Range([int]$Segment.Start, [int]$Segment.End)
  $expectedText = Normalize-Text ([string]$Segment.Text)
  $expectedPage = 0
  try {
    $startRange = $Doc.Range([int]$Segment.Start, [int]([Math]::Min([int]$Segment.Start + 1, [int]$Segment.End)))
    $expectedPage = [int]$startRange.Information($WdActiveEndPageNumber)
  } catch {}
  $range.Select()
  Start-Sleep -Milliseconds 120

  $selectedText = Normalize-Text ([string]$App.Selection.Range.Text)
  if ($selectedText -eq $expectedText) {
    return [pscustomobject]@{ Range = $range; ExpectedPage = $expectedPage; Fallback = $false }
  }

  $docEnd = [int]$Doc.Content.End
  $segmentStart = [int]$Segment.Start
  $segmentEnd = [int]$Segment.End
  $scopeStart = if ($null -ne $Segment.ScopeStart) { [int]$Segment.ScopeStart } else { $segmentStart }
  $scopeEnd = if ($null -ne $Segment.ScopeEnd) { [int]$Segment.ScopeEnd } else { $segmentEnd }
  $attempts = @(
    @([Math]::Max(0, $segmentStart), [Math]::Min($docEnd, $segmentEnd + 2)),
    @([Math]::Max(0, $scopeStart), [Math]::Min($docEnd, $scopeEnd)),
    @([Math]::Max(0, $scopeStart - 2), [Math]::Min($docEnd, $scopeEnd + 2))
  )
  foreach ($attempt in $attempts) {
    if ([int]$attempt[1] -le [int]$attempt[0]) { continue }
    foreach ($text in (Get-SearchTexts ([string]$Segment.Text))) {
      try {
        $search = $Doc.Range([int]$attempt[0], [int]$attempt[1])
        $find = $search.Find
        try { $find.ClearFormatting() | Out-Null } catch {}
        if ($find.Execute($text)) {
          $foundStart = [int]$search.Start
          $foundEnd = [int]$search.End
          if ($foundStart -lt [int]$attempt[0] -or $foundEnd -gt [int]$attempt[1]) { continue }
          if ($foundStart -lt ($scopeStart - 2) -or $foundEnd -gt ($scopeEnd + 2)) { continue }
          $search.Select()
          Start-Sleep -Milliseconds 120
          $selectedText = Normalize-Text ([string]$App.Selection.Range.Text)
          if ($selectedText -eq $expectedText) {
            $foundPage = 0
            try {
              $foundStartRange = $Doc.Range($foundStart, [int]([Math]::Min($foundStart + 1, $foundEnd)))
              $foundPage = [int]$foundStartRange.Information($WdActiveEndPageNumber)
            } catch {}
            if ($foundPage -gt 0) { $expectedPage = $foundPage }
            return [pscustomobject]@{ Range = $search; ExpectedPage = $expectedPage; Fallback = $true }
          }
        }
      } catch {}
    }
  }

  return [pscustomobject]@{ Range = $range; ExpectedPage = $expectedPage; Fallback = $false }
}

function New-WpsApplication([string[]]$ProgIds = @("KWPS.Application", "Word.Application")) {
  foreach ($progId in $ProgIds) {
    try {
      $app = New-Object -ComObject $progId
      if ($null -ne $app) {
        return [pscustomobject]@{ App = $app; ProgId = $progId }
      }
    } catch {
      continue
    }
  }
  throw "No WPS/Word COM automation interface is available."
}

function Close-AutomationDocument($Doc, $App) {
  if ($Doc -ne $null) {
    try { $Doc.Close($false) } catch {}
  }
  if ($App -ne $null) {
    try { $App.Quit() } catch {}
  }
}

if (!(Test-Path -LiteralPath $DocumentPath)) {
  throw "Test document does not exist: $DocumentPath"
}

$resolvedDocument = (Resolve-Path -LiteralPath $DocumentPath).Path
$automationOrder = if ($PreferWordAutomation) { @("Word.Application", "KWPS.Application") } else { @("KWPS.Application", "Word.Application") }
$wps = New-WpsApplication -ProgIds $automationOrder
$app = $wps.App
$doc = $null

try {
  $app.Visible = $false
  $doc = $app.Documents.Open($resolvedDocument)
  Start-Sleep -Milliseconds 800

  $segments = Get-ParagraphSegments -Doc $doc
  if ($segments.Count -eq 0 -and $wps.ProgId -eq "KWPS.Application") {
    Close-AutomationDocument -Doc $doc -App $app
    $doc = $null
    $wps = New-WpsApplication -ProgIds @("Word.Application")
    $app = $wps.App
    $app.Visible = $false
    $doc = $app.Documents.Open($resolvedDocument)
    Start-Sleep -Milliseconds 800
    $segments = Get-ParagraphSegments -Doc $doc
  }
  if ($segments.Count -eq 0) {
    throw "The document has no readable segments to test."
  }

  $fullDoc = Test-Segments -Doc $doc -App $app -Segments $segments -Name "continuous-full-document" -MinPagesRequired $MinPages -Limit $MaxChecks
  $scenarios = New-Object System.Collections.Generic.List[object]
  $scenarios.Add($fullDoc)

  foreach ($page in $ScenarioPages) {
    $pageStart = Get-PageStart -Doc $doc -App $app -Page $page
    if ($null -eq $pageStart) { continue }
    $pageEnd = Get-PageEnd -Doc $doc -App $app -Page $page
    $continuousSegments = Select-ScenarioSegments -Segments $segments -Start $pageStart -Limit 8
    if ($continuousSegments.Count -gt 0) {
      $scenarios.Add((Test-Segments -Doc $doc -App $app -Segments $continuousSegments -Name ("continuous-from-page-" + $page) -MinPagesRequired 1 -Limit 0))
    }
    $pageSegments = Select-ScenarioSegments -Segments $segments -Start $pageStart -End $pageEnd -Limit 0
    if ($pageSegments.Count -gt 0) {
      $scenarios.Add((Test-Segments -Doc $doc -App $app -Segments $pageSegments -Name ("current-page-" + $page) -MinPagesRequired 1 -Limit 0))
    }
  }

  $failed = @($scenarios | Where-Object { -not $_.Ok })
  $summaryData = @{
    Ok                 = ($failed.Count -eq 0)
    Document           = $resolvedDocument
    Automation         = $wps.ProgId
    TotalSegments      = $segments.Count
    FullDocumentChecks = $fullDoc.CheckedSegments
    FullDocumentPages  = $fullDoc.CoveredPages
    ScenarioCount      = $scenarios.Count
    FailedScenarios    = $failed.Count
    ScenarioSummary    = @($scenarios | ForEach-Object {
      [pscustomobject]@{
        Name            = $_.Name
        Ok              = $_.Ok
        CheckedSegments = $_.CheckedSegments
        CoveredPages    = $_.CoveredPages
        FailedCount     = $_.FailedCount
        FirstFailure    = $_.FirstFailure
      }
    })
  }
  if (!$SummaryOnly) {
    $summaryData.Scenarios = $scenarios
  }
  $summary = [pscustomobject]$summaryData

  $summary | ConvertTo-Json -Depth 6
  if (!$summary.Ok) {
    throw "Selection consistency test failed: scenarios $($scenarios.Count), failed $($failed.Count)."
  }
} finally {
  if (-not $KeepWpsOpen) {
    Close-AutomationDocument -Doc $doc -App $app
  }
}
