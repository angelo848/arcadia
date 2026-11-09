# Spec traceability check (SDD).
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/<feature>
# Checks, for one spec folder:
#   1. every requirement in requirements.md is covered by a task in tasks.md (once tasks exist);
#   2. every non-manual, non-superseded requirement is cited by at least one test (*_test.go, *.test.ts, *.test.tsx);
#      a requirement is superseded when its section (from its '### ID' heading to the next heading) contains
#      "substituido por" (accented in the docs); it is listed as 'substituido' and never counted as a problem;
#   3. no test or task cites an ID that does not exist in any requirements.md.
# Exit 0 = traceable, 1 = gaps found, 2 = usage error.
param(
    [Parameter(Mandatory = $true)][string]$Spec,
    [switch]$SkipTests   # use before the test-writer step: checks only requirements <-> tasks
)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$root = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$specDir = if ([System.IO.Path]::IsPathRooted($Spec)) { $Spec } else { Join-Path $root $Spec }
$reqFile = Join-Path $specDir 'requirements.md'
if (-not (Test-Path -LiteralPath $reqFile)) { Write-Output "requirements.md nao encontrado em $specDir"; exit 2 }

$idPattern = '\b([A-Z]{2,4}-\d{3})\b'

function Read-Requirements([string]$file) {
    $reqs = [ordered]@{}
    $current = $null
    foreach ($line in [System.IO.File]::ReadAllLines($file, [System.Text.Encoding]::UTF8)) {
        if ($line -match '^###\s+([A-Z]{2,4}-\d{3})\b') {
            $current = $Matches[1]
            if ($reqs.Contains($current)) { Write-Output "ID duplicado em ${file}: $current"; exit 2 }
            $reqs[$current] = @{ Manual = $false; Superseded = $false }
        } elseif ($line -match '^#{1,6}\s') {
            $current = $null   # any other heading ends the requirement's section
        } elseif ($current) {
            # ASCII-only patterns: PS 5.1 reads BOM-less .ps1 as ANSI
            if ($line -match 'verifica.{1,2}.{1,2}o:\s*manual') { $reqs[$current].Manual = $true }
            if ($line -match 'substitu.{1,2}do\s+por') { $reqs[$current].Superseded = $true }
        }
    }
    return $reqs
}

$reqs = Read-Requirements $reqFile

# IDs known across all specs (a test may legitimately cite a requirement from another phase).
$allIds = @{}
Get-ChildItem -Path (Join-Path $root 'docs/specs') -Recurse -Filter 'requirements.md' | ForEach-Object {
    foreach ($k in (Read-Requirements $_.FullName).Keys) { $allIds[$k] = $true }
}

$problems = New-Object System.Collections.Generic.List[string]

# 1. requirements <-> tasks
$taskFile = Join-Path $specDir 'tasks.md'
$taskIds = @{}
$hasTasks = $false
if (Test-Path -LiteralPath $taskFile) {
    $inFence = $false
    foreach ($line in [System.IO.File]::ReadAllLines($taskFile, [System.Text.Encoding]::UTF8)) {
        if ($line -match '^\s*```') { $inFence = -not $inFence; continue }
        if ($inFence) { continue }
        if ($line -match '^##\s+T\d+') { $hasTasks = $true }
        if ($hasTasks) { foreach ($m in [regex]::Matches($line, $idPattern)) { $taskIds[$m.Groups[1].Value] = $true } }
    }
}
if ($hasTasks) {
    foreach ($id in $reqs.Keys) { if (-not $taskIds[$id]) { $problems.Add("sem tarefa:   $id") } }
    foreach ($id in $taskIds.Keys) { if (-not $allIds[$id]) { $problems.Add("tarefa cita ID inexistente: $id") } }
} else {
    Write-Output "(tasks.md ainda sem tarefas; cobertura requisito->tarefa nao verificada)"
}

# 2/3. requirements <-> tests
$cited = @{}
if (-not $SkipTests) {
    $testFiles = Get-ChildItem -Path $root -Recurse -File -Include '*_test.go', '*.test.ts', '*.test.tsx' -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -notmatch '[\\/](node_modules|\.git|vendor)[\\/]' }
    foreach ($f in $testFiles) {
        $rel = $f.FullName.Substring($root.Length + 1) -replace '\\', '/'
        $text = [System.IO.File]::ReadAllText($f.FullName)
        foreach ($m in [regex]::Matches($text, $idPattern)) {
            $id = $m.Groups[1].Value
            if (-not $allIds[$id]) { $problems.Add("teste cita ID inexistente: $id ($rel)"); continue }
            if (-not $cited[$id]) { $cited[$id] = New-Object System.Collections.Generic.List[string] }
            if (-not $cited[$id].Contains($rel)) { $cited[$id].Add($rel) }
        }
    }
    foreach ($id in $reqs.Keys) {
        if (-not $reqs[$id].Manual -and -not $reqs[$id].Superseded -and -not $cited[$id]) { $problems.Add("sem teste:    $id") }
    }
}

# Report
Write-Output "Spec: $($specDir.Substring($root.Length + 1) -replace '\\','/')"
foreach ($id in $reqs.Keys) {
    $status = if ($reqs[$id].Superseded) { 'substituido' } elseif ($reqs[$id].Manual) { 'manual' } elseif ($SkipTests) { '-' } elseif ($cited[$id]) { ($cited[$id] -join ', ') } else { 'SEM TESTE' }
    Write-Output ("  {0,-8} {1}" -f $id, $status)
}
$superseded = @($reqs.Keys | Where-Object { $reqs[$_].Superseded })
if ($superseded.Count -gt 0) { Write-Output "Substituidos (teste nao exigido): $($superseded -join ', ')" }
if ($problems.Count -eq 0) { Write-Output "OK: $($reqs.Count) requisitos rastreados."; exit 0 }
Write-Output "`n$($problems.Count) problema(s):"
$problems | Sort-Object -Unique | ForEach-Object { Write-Output "  $_" }
exit 1
