# 0005 — Rastreamento de tempo em camadas (Job Object + watcher + sinais de loja)

- **Status:** aceito
- **Data:** 2026-11-07

## Contexto
Muitos jogos abrem por um exe "lançador" que inicia o jogo real e fecha; outros são iniciados por clientes já abertos
(Steam, Epic). Rastrear só o PID inicial conta segundos em vez de horas.

## Decisão
1. **Job Object** com IOCP: o jogo nasce suspenso, entra no job e só então é retomado; todo descendente herda o job.
   A sessão acaba em `ACTIVE_PROCESS_ZERO` + período de carência (10–30 s). Sem `KILL_ON_JOB_CLOSE`.
2. **Watcher Toolhelp32** (~1,5 s, sem admin) adota processos fora da árvore que casem com regras do jogo
   (exe configurado/aprendido, pasta de instalação), identificados por (PID, creation time).
3. **Sinais de loja**, começando pelo registro da Steam (`Apps\<appid>\Running`).
Estado da sessão numa máquina de estados pura (Idle → Launching → Running ⇄ Paused → Grace → Ended/Failed),
com tempo via `domain.Clock` e heartbeat a cada 30 s no banco.

## Alternativas consideradas
- **Só o PID inicial** — falha no caso principal que motivou o projeto.
- **WMI `Win32_ProcessStartTrace` / ETW** — exigem admin; ETW fica como upgrade opcional.

## Consequências
- ✅ Funciona para lançadores encadeados, relançamentos e jogos de loja, sem privilégio de admin.
- ⚠️ Código Win32 delicado: só o tracker-specialist escreve em `internal/tracker/` (ADR 0007).
- ⚠️ Heurística de descoberta pode errar; o usuário confirma e a regra é salva.
