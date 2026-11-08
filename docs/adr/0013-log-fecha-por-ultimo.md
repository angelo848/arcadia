# 0013 — O log fecha por último no shutdown

- **Status:** aceito
- **Data:** 2026-11-08
- **Complementa:** [0003](0003-agente-separado-da-ui.md)

## Contexto
Agente e UI logam com `slog` JSON sobre `lumberjack` (v2.2.1). Duas propriedades da biblioteca afetam o encerramento:
um `Write` depois de `Close` reabre o arquivo em silêncio (o handle fica sem dono), e a goroutine de limpeza
(`millRun`), iniciada no primeiro `Write`, não é encerrada pelo `Close`. Componentes que fecham depois do log (watcher
de `WM_ENDSESSION`, mutex de instância única) recebem o logger e podem registrar mensagens.

## Decisão
O fechamento do log é **sempre o último passo** do shutdown, depois de todo componente que recebeu o `*slog.Logger`.
A escrita é síncrona e sem buffer, então o que já foi logado está no sistema operacional mesmo que o processo seja
encerrado antes do `Close`. A goroutine `millRun` é aceita como exceção à regra "toda goroutine tem dono e condição de
saída" (uma por processo); testes com `goleak` a ignoram via `IgnoreTopFunction`.

## Alternativas consideradas
- **Fechar o log antes do watcher** — um log no timeout de `WM_ENDSESSION` reabriria o arquivo e vazaria o handle.
- **Rotator próprio** — elimina a goroutine, mas são ~80 linhas de I/O no Windows (rename com arquivo aberto, poda)
  para economizar uma goroutine parada.

## Consequências
- ✅ Nenhum handle de log reaberto no shutdown; ordem única e fácil de revisar.
- ⚠️ Mensagens após `ShutdownDone` são melhor esforço: no logoff o Windows pode encerrar o processo logo em seguida.
- ⚠️ Trocar ou atualizar o lumberjack exige rever esta exceção.
