# Batch generation of 16 VexaVPN cards via KIE (image-to-image from an approved anchor).
# Single style: keep the anchor's background/palette/badge, change only the icon + title.
# Titles are read from cards.json as UTF-8 (avoids PS 5.1 mojibake in this script).
#
# Usage:
#   $env:KIE_KEY="..."; ./kie_batch.ps1 create   # create all tasks, save taskIds
#   $env:KIE_KEY="..."; ./kie_batch.ps1 fetch     # poll saved tasks, download PNGs
param([Parameter(Mandatory=$true)][ValidateSet("create","fetch")][string]$Mode)

$ErrorActionPreference = "Stop"
$key = $env:KIE_KEY
if (-not $key) { throw "set KIE_KEY" }
$base    = "https://api.kie.ai/api/v1/gpt4o-image"
$headers = @{ Authorization = "Bearer $key" }
$root    = "C:\projects\vpnbottg\assets\promo"
$outDir  = Join-Path $root "cards"
$anchor  = "https://tempfile.redpandaai.co/kieai/11340960/images/vexa/anchor_dark.png"
$taskFile = Join-Path $root "_batch_tasks.txt"

$style = "Keep EXACTLY the same visual style as the reference image: dark navy-blue background, cold blue glow, glossy 3D central icon, subtle geometric accents, same lighting, and the same bottom rounded pill badge reading '@vexa_vpn_bot'. Premium minimalist SaaS cover card, square 1:1 composition, centered. Change ONLY two things: (1) the central 3D icon, (2) the bold title text near the top. Keep everything else identical."

# Read screens from UTF-8 JSON.
$json = [System.IO.File]::ReadAllText((Join-Path $root "cards.json"), [System.Text.Encoding]::UTF8)
$screens = $json | ConvertFrom-Json

if ($Mode -eq "create") {
  New-Item -ItemType Directory -Force -Path $outDir | Out-Null
  Remove-Item $taskFile -ErrorAction SilentlyContinue
  foreach ($s in $screens) {
    $prompt = "$style Central 3D icon: $($s.icon). Bold title text, single short line, in Russian, exactly: `"$($s.title)`". Do NOT add any subtitle or other text besides the title and the @vexa_vpn_bot pill."
    $body = @{ prompt = $prompt; size = "1:1"; nVariants = 1; isEnhance = $false; enableFallback = $true; filesUrl = @($anchor) } | ConvertTo-Json
    try {
      $resp = Invoke-RestMethod -Uri "$base/generate" -Method Post -Headers $headers -ContentType "application/json" -Body $body
      $task = $resp.data.taskId
      if ($task) {
        "$($s.name)=$task" | Add-Content -Path $taskFile -Encoding UTF8
        Write-Host ("created {0,-15} {1}" -f $s.name, $task)
      } else {
        Write-Host ("NO TASKID {0}: {1}" -f $s.name, ($resp | ConvertTo-Json -Depth 5))
      }
    } catch {
      Write-Host ("ERR create {0}: {1}" -f $s.name, $_.Exception.Message)
    }
    Start-Sleep -Seconds 2
  }
  Write-Host "=== all tasks created, saved to $taskFile ==="
  return
}

# fetch mode
if (-not (Test-Path $taskFile)) { throw "no $taskFile - run create first" }
$pairs = @{}
foreach ($line in (Get-Content $taskFile -Encoding UTF8)) {
  if ($line -match "^(.+?)=(.+)$") { $pairs[$matches[1]] = $matches[2] }
}
$pending = [System.Collections.ArrayList]@($pairs.Keys)
for ($round = 1; $round -le 60 -and $pending.Count -gt 0; $round++) {
  Start-Sleep -Seconds 10
  $done = @()
  foreach ($name in $pending) {
    $task = $pairs[$name]
    try {
      $r = Invoke-RestMethod -Uri "$base/record-info?taskId=$task" -Method Get -Headers $headers
      $st = $r.data.status
      $url = $null
      if ($r.data.response) {
        if ($r.data.response.resultUrls) { $url = $r.data.response.resultUrls[0] }
        elseif ($r.data.response.result_urls) { $url = $r.data.response.result_urls[0] }
      }
      if ($url) {
        $out = Join-Path $outDir "$name.png"
        Invoke-WebRequest -Uri $url -OutFile $out
        Write-Host ("[r{0}] OK   {1,-15} {2} bytes" -f $round, $name, (Get-Item $out).Length)
        $done += $name
      } elseif ($st -in @("GENERATE_FAILED","FAILED","CREATE_TASK_FAILED")) {
        Write-Host ("[r{0}] FAIL {1}: {2}" -f $round, $name, ($r.data | ConvertTo-Json -Depth 5))
        $done += $name
      } else {
        Write-Host ("[r{0}] .... {1,-15} {2}" -f $round, $name, $st)
      }
    } catch {
      Write-Host ("[r{0}] ERR  {1}: {2}" -f $round, $name, $_.Exception.Message)
    }
  }
  foreach ($d in $done) { $pending.Remove($d) }
}
if ($pending.Count -gt 0) { Write-Host ("STILL PENDING: {0}" -f ($pending -join ", ")) }
Write-Host "=== fetch done ==="
