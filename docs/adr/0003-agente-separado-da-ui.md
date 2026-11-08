# 0003 — Agente residente separado da UI, IPC por named pipe

- **Status:** aceito
- **Data:** 2026-11-07

## Contexto
O tempo de jogo não pode depender da janela principal: abrir, fechar, atualizar ou travar a UI não deve afetar
a contagem. Atalhos globais e polling de controle precisam continuar com a UI fechada.

## Decisão
Teremos dois executáveis: `arcadia-agent.exe` (bandeja; lança jogos, rastreia, atalhos, dono das tabelas de sessão)
e `arcadia.exe` (UI Wails). Eles conversam por **named pipe** por usuário (`go-winio`, ACL só do usuário atual) com
**JSON-RPC 2.0 em NDJSON**. A UI inicia o agente desacoplado se ele não estiver rodando; fechar a UI nunca o encerra.

## Alternativas consideradas
- **Processo único com "fechar para a bandeja"** — um crash da UI derruba a sessão em andamento.
- **HTTP/WebSocket em localhost** — abre porta de rede e exige autenticação própria.
- **gRPC** — mais peso e geração de código sem ganho para poucos métodos locais.

## Consequências
- ✅ Rastreamento isolado e com consumo mínimo; a UI pode ser reiniciada a qualquer momento.
- ⚠️ Protocolo versionado entre os dois binários; os tipos ficam em `internal/ipc`.
- ⚠️ Dois processos escrevem no mesmo SQLite (ver ADR 0004).
