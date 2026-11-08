# PreToolUse (Edit|Write|MultiEdit|NotebookEdit): enforce per-agent write scopes.
#   -Allow  regex the project-relative path MUST match (empty = anything)
#   -Deny   regex the project-relative path must NOT match (empty = nothing)
#   -Role   agent name, used in the message
param(
    [string]$Allow = '',
    [string]$Deny = '',
    [string]$Role = 'agente'
)
. "$PSScriptRoot\common.ps1"

$hook = Read-HookInput
if (-not $hook) { Block "${Role}: entrada do hook ilegivel; escrita bloqueada por seguranca." }
$path = $hook.tool_input.file_path
if (-not $path) { $path = $hook.tool_input.notebook_path }
if (-not $path) { exit 0 }

$rel = Get-ProjectRelativePath $path
if (-not $rel) { Block "${Role}: escrita fora do projeto nao permitida ($path)." }
if ($Allow -and ($rel -notmatch $Allow)) { Block "${Role}: nao pode escrever em '$rel' (permitido: $Allow)." }
if ($Deny -and ($rel -match $Deny)) { Block "${Role}: nao pode escrever em '$rel' (bloqueado: $Deny)." }
exit 0
