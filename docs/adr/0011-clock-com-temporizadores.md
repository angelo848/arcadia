# 0011 — `domain.Clock` com temporizadores para toda espera

- **Status:** aceito
- **Data:** 2026-11-08

## Contexto
Heartbeat do IPC, backoff de reconexão e, na fase 3, carência da sessão, heartbeat no banco e timeout de lançamento
dependem de esperar tempo. Testar isso com `time.Sleep` é lento e instável, e as convenções o proíbem.

## Decisão
`domain.Clock` expõe `Now() time.Time` e `After(d) <-chan time.Time`. Todo código que espera tempo usa `clock.After`
(nunca `time.After`/`time.Sleep`/`time.NewTicker` direto). `FakeClock` oferece `Advance(d)`, que dispara os waiters
vencidos, e `BlockUntilWaiters(ctx, n)`, para o teste saber que o código já está esperando antes de avançar o relógio.

## Alternativas consideradas
- **Biblioteca externa (ex.: `benbjohnson/clock`)** — funciona, mas adiciona dependência para ~80 linhas de código.
- **Intervalos configuráveis bem curtos nos testes** — ainda depende do relógio real e gera testes intermitentes.

## Consequências
- ✅ Testes de tempo determinísticos e instantâneos.
- ⚠️ Revisões devem barrar `time.After`, `time.Sleep` e tickers em lógica de produção testável.
