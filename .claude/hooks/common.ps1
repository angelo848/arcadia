# Shared helpers for Claude Code hooks (Windows PowerShell 5.1+).
# Hook contract: JSON on stdin; exit 2 + stderr = block / feed back to Claude; exit 0 = ok.

[Console]::InputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = 'Continue'

function Read-HookInput {
    $raw = [Console]::In.ReadToEnd()
    if ([string]::IsNullOrWhiteSpace($raw)) { return $null }
    try { return $raw | ConvertFrom-Json } catch { return $null }
}

# Runs a native command and returns stdout+stderr as plain text (no PowerShell ErrorRecord noise).
function Invoke-Native([string]$exe, [string[]]$arguments) {
    $out = & $exe @arguments 2>&1 | ForEach-Object { "$_" } | Out-String
    return $out
}

function Get-ProjectDir {
    if ($env:CLAUDE_PROJECT_DIR) { return [System.IO.Path]::GetFullPath($env:CLAUDE_PROJECT_DIR).TrimEnd('\', '/') }
    return [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..')).TrimEnd('\', '/')
}

# Accepts Windows paths and Git Bash style paths (/c/Users/...).
function ConvertTo-WindowsPath([string]$path) {
    if ($path -match '^/([a-zA-Z])/(.*)$') { $path = "$($Matches[1]):/$($Matches[2])" }
    if (-not [System.IO.Path]::IsPathRooted($path)) { $path = Join-Path (Get-ProjectDir) $path }
    return [System.IO.Path]::GetFullPath($path)
}

# Project-relative path with forward slashes, or $null if outside the project.
function Get-ProjectRelativePath([string]$path) {
    $root = Get-ProjectDir
    $full = ConvertTo-WindowsPath $path
    if (-not $full.StartsWith($root + '\', [System.StringComparison]::OrdinalIgnoreCase)) { return $null }
    return ($full.Substring($root.Length + 1) -replace '\\', '/')
}

function Find-GoTool([string]$name) {
    $cmd = Get-Command $name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $candidates = @()
    if ($env:GOROOT) { $candidates += (Join-Path $env:GOROOT "bin\$name.exe") }
    $candidates += (Join-Path $env:ProgramFiles "Go\bin\$name.exe")
    foreach ($c in $candidates) { if (Test-Path -LiteralPath $c) { return $c } }
    return $null
}

function Block([string]$message) {
    [Console]::Error.WriteLine($message)
    exit 2
}
