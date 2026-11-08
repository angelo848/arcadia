# Stop hook for code-writing subagents: run go test and keep the agent working while it fails.
# Gives up after -MaxBlocks consecutive blocks so a stubborn failure cannot loop forever.
param(
    [string]$Packages = './...',
    [int]$MaxBlocks = 3
)
. "$PSScriptRoot\common.ps1"

$hook = Read-HookInput
$root = Get-ProjectDir
if (-not (Test-Path -LiteralPath (Join-Path $root 'go.mod'))) { exit 0 }

$go = Find-GoTool 'go'
if (-not $go) {
    [Console]::Error.WriteLine('go nao encontrado; go test nao foi executado.')
    exit 1
}

$sessionId = if ($hook -and $hook.session_id) { $hook.session_id } else { 'nosession' }
$counter = Join-Path $env:TEMP "arcadia-gotest-$sessionId.count"

Push-Location $root
$env:CGO_ENABLED = '0'
$out = Invoke-Native $go @('test', $Packages)
$code = $LASTEXITCODE
Pop-Location

if ($code -eq 0) {
    Remove-Item -LiteralPath $counter -ErrorAction SilentlyContinue
    exit 0
}

$n = 0
if ($hook -and $hook.stop_hook_active -and (Test-Path -LiteralPath $counter)) {
    $n = [int](Get-Content -LiteralPath $counter -Raw)
}
$n++

if ($n -gt $MaxBlocks) {
    Remove-Item -LiteralPath $counter -ErrorAction SilentlyContinue
    $msg = "go test $Packages ainda falha apos $MaxBlocks tentativas; o agente foi liberado. Verifique as falhas."
    @{ systemMessage = $msg } | ConvertTo-Json -Compress
    exit 0
}

Set-Content -LiteralPath $counter -Value $n -Encoding ascii
$tail = ($out -split "`r?`n" | Select-Object -Last 150) -join "`n"
Block "go test $Packages falhou (bloqueio $n/$MaxBlocks). Corrija antes de terminar; se a falha estiver num teste que voce nao pode editar, explique no relatorio final.`n$tail"
