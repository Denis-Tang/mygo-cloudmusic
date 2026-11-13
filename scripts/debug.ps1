# Runs the app with the control API, and says where it is: the token, the
# log, and the code it exited with. Headless unless -Visible, so a test can
# drive it on a machine nobody is looking at.
param(
  [string]$Exe = "build/release-local/windows-amd64/MyGo 云音乐.exe",
  [switch]$Visible,
  [switch]$Restart
)
$ErrorActionPreference = "Stop"
$env:MYGO_CONTROL = "1"
if (-not $Visible) { $env:MYGO_HEADLESS = "1" }

$data = Join-Path $env:APPDATA "MyGo 云音乐"
$control = Join-Path $data "control.json"
$logs = Join-Path $data "logs"

do {
  Remove-Item -LiteralPath $control -ErrorAction SilentlyContinue
  $proc = Start-Process -FilePath $Exe -PassThru
  Write-Host "started $($proc.Id)"

  foreach ($i in 0..50) {
    if (Test-Path $control) { break }
    Start-Sleep -Milliseconds 200
  }
  if (Test-Path $control) {
    $j = Get-Content -LiteralPath $control -Raw | ConvertFrom-Json
    Write-Host "control http://127.0.0.1:$($j.port)  token $($j.token)"
    Write-Host "state   curl -s `"http://127.0.0.1:$($j.port)/state?token=$($j.token)`""
  } else {
    Write-Warning "no control.json: the app may have failed before serving"
  }

  $proc.WaitForExit()
  New-Item -ItemType Directory -Force -Path $logs | Out-Null
  $line = "exit {0} at {1}" -f $proc.ExitCode, (Get-Date -Format o)
  Add-Content -LiteralPath (Join-Path $logs "exit.log") -Value $line
  Write-Host $line
  if (Test-Path $logs) {
    Get-ChildItem -LiteralPath $logs -Filter "crash-*.log" | Sort-Object LastWriteTime | Select-Object -Last 3 | ForEach-Object {
      Write-Host "crash: $($_.FullName)"
    }
  }
} while ($Restart)
