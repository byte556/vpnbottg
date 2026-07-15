# Генератор карточек через KIE.ai (gpt4o-image). Синий SaaS-стиль VexaVPN.
# Использование:
#   $env:KIE_KEY="xxx"; ./kie_gen.ps1 -Prompt "<prompt>" -Out "out.png" [-Size 2:3]
param(
  [Parameter(Mandatory=$true)][string]$Prompt,
  [Parameter(Mandatory=$true)][string]$Out,
  [string]$Size = "2:3"
)
$ErrorActionPreference = "Stop"
$key = $env:KIE_KEY
if (-not $key) { throw "set KIE_KEY env var" }
$base = "https://api.kie.ai/api/v1/gpt4o-image"
$headers = @{ Authorization = "Bearer $key" }

$body = @{ prompt = $Prompt; size = $Size; nVariants = 1; isEnhance = $false; enableFallback = $true } | ConvertTo-Json
Write-Host ">> createTask ($Size)"
$resp = Invoke-RestMethod -Uri "$base/generate" -Method Post -Headers $headers -ContentType "application/json" -Body $body
$task = $resp.data.taskId
if (-not $task) { $resp | ConvertTo-Json -Depth 6 | Write-Host; throw "no taskId" }
# taskId сразу на диск рядом с -Out, чтобы не потерять при фоновом таймауте
Set-Content -Path ("$Out.taskid") -Value $task -Encoding ascii
Write-Host ">> taskId=$task ; polling..."

for ($i=1; $i -le 90; $i++) {
  Start-Sleep -Seconds 6
  $r = Invoke-RestMethod -Uri "$base/record-info?taskId=$task" -Method Get -Headers $headers
  $st = $r.data.status
  $url = $null
  if ($r.data.response) {
    if ($r.data.response.resultUrls) { $url = $r.data.response.resultUrls[0] }
    elseif ($r.data.response.result_urls) { $url = $r.data.response.result_urls[0] }
  }
  Write-Host ("  [{0}] status={1}" -f $i, $st)
  if ($url) {
    Write-Host ">> done: $url"
    Invoke-WebRequest -Uri $url -OutFile $Out
    Write-Host (">> saved: {0} ({1} bytes)" -f $Out, (Get-Item $Out).Length)
    exit 0
  }
  if ($st -in @("GENERATE_FAILED","FAILED","CREATE_TASK_FAILED")) {
    $r | ConvertTo-Json -Depth 6 | Write-Host
    throw "generation failed"
  }
}
throw "timeout"
