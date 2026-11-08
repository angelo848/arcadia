# PostToolUse (Edit|Write|MultiEdit): format every edited .go file with gofmt -w.
. "$PSScriptRoot\common.ps1"

$hook = Read-HookInput
$path = $hook.tool_input.file_path
if (-not $path -or -not $path.EndsWith('.go')) { exit 0 }

$path = ConvertTo-WindowsPath $path
if (-not (Test-Path -LiteralPath $path)) { exit 0 }

$gofmt = Find-GoTool 'gofmt'
if (-not $gofmt) {
    [Console]::Error.WriteLine('gofmt nao encontrado (instale o Go ou adicione ao PATH); arquivo nao formatado.')
    exit 1
}

$out = Invoke-Native $gofmt @('-l', '-w', $path)
if ($LASTEXITCODE -ne 0) {
    Block "gofmt falhou em ${path} (provavel erro de sintaxe):`n$out"
}
exit 0
