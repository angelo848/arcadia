# PreToolUse (Bash): allow only simple commands that match -Allow; no chaining, pipes or redirection.
param(
    [Parameter(Mandatory = $true)][string]$Allow,
    [string]$Role = 'agente'
)
. "$PSScriptRoot\common.ps1"

$hook = Read-HookInput
if (-not $hook) { Block "${Role}: entrada do hook ilegivel; comando bloqueado por seguranca." }
$cmd = ([string]$hook.tool_input.command).Trim()
if (-not $cmd) { exit 0 }

if ($cmd -match '[;&|<>`]|\$\(|\r|\n') {
    Block "${Role}: use um comando simples, sem encadeamento, pipe, redirecionamento ou subshell: $cmd"
}
if ($cmd -notmatch $Allow) {
    Block "${Role}: comando nao permitido para este agente: $cmd (permitido: $Allow)"
}
exit 0
