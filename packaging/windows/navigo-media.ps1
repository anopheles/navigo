# Sends next/previous/toggle to navigo's mpv only, through Windows' media sessions,
# so a browser tab that is also playing is left alone.
param([ValidateSet('next','previous','toggle','list')][string]$Action = 'list')
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
  $_.Name -eq 'AsTask' -and $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and
  $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1' } | Select-Object -First 1
function Await($op, [Type]$t) { $task = $asTask.MakeGenericMethod($t).Invoke($null, @($op)); [void]$task.Wait(3000); $task.Result }
[void][Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager, Windows.Media.Control, ContentType = WindowsRuntime]
$mgr = Await ([Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync()) ([Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager])
$sessions = $mgr.GetSessions()
if ($Action -eq 'list') {
  foreach ($s in $sessions) { "$($s.SourceAppUserModelId) status=$($s.GetPlaybackInfo().PlaybackStatus)" }
  return
}
$s = $sessions | Where-Object { $_.SourceAppUserModelId -match 'mpv' } | Select-Object -First 1
if (-not $s) { return }
switch ($Action) {
  'next'     { [void](Await ($s.TrySkipNextAsync()) ([bool])) }
  'previous' { [void](Await ($s.TrySkipPreviousAsync()) ([bool])) }
  'toggle'   { [void](Await ($s.TryTogglePlayPauseAsync()) ([bool])) }
}
