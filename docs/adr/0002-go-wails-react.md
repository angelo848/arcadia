# 0002 — Go + Wails v2 + React como stack

- **Status:** aceito
- **Data:** 2026-11-07

## Contexto
Precisamos de acesso fino à API Win32 (Job Objects, IOCP, Toolhelp, RegisterHotKey, XInput), um processo residente
leve e uma UI rica (grade de capas, vídeo, temas). A primeira proposta foi Rust + Tauri.

## Decisão
Usaremos **Go 1.27** com `golang.org/x/sys/windows` no backend, **Wails v2** (WebView2) como shell desktop e
**React + TypeScript + Vite** na interface. Migração para Wails v3 quando estiver estável.

## Alternativas consideradas
- **Rust + Tauri** — mais enxuto, mas curva de aprendizado alta para o ritmo do projeto.
- **C#/.NET 10 (WPF/Avalonia)** — ótimo no Windows; binários maiores e vídeo/temas mais trabalhosos.
- **Java 25 + GraalVM** — FFM trabalhoso para Win32 e sem equivalente maduro ao Wails.

## Consequências
- ✅ Binário único sem runtime, build simples, goroutines baratas para vigiar processos.
- ✅ Bindings Go↔TS gerados pelo Wails.
- ⚠️ APIs fora de `x/sys/windows` (user32, xinput) exigem wrappers próprios em `internal/winapi`.
- ⚠️ Disciplina obrigatória com handles e goroutines (o GC não fecha handles do Windows).
