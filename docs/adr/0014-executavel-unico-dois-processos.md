# 0014 — Executável único com dois processos

- **Status:** aceito; a parte de início com o Windows foi substituída pela [0024](0024-agente-sempre-como-administrador.md)
- **Data:** 2026-11-08
- **Substitui em parte:** [0003](0003-agente-separado-da-ui.md) (só a parte "dois executáveis")

## Contexto
Distribuir `arcadia.exe` e `arcadia-agent.exe` confunde o usuário: ele quer um único programa que abre tudo.
O isolamento do rastreamento (ADR 0003) continua obrigatório, e também é obrigatório que "Sair" na bandeja
feche a UI e que a UI nunca fique viva sem o agente. Spec: [docs/specs/single-exe](../specs/single-exe/requirements.md).

## Decisão
Haverá um só executável, `arcadia.exe`, rodando em **dois processos**:
- `arcadia.exe --agent` é o agente: bandeja, IPC, store, migrações e tempo de jogo, sem Wails.
- `arcadia.exe` sem a flag é a UI, de instância única. Ela garante que o agente esteja vivo, relançando o próprio
  exe com `--agent` desacoplado. Se o agente cair sem aviso, a UI o relança, com limite de tentativas.
- Antes de encerrar, o agente envia `agent.shutdown` às UIs conectadas, e a UI fecha sem relançá-lo.
- O agente cria a entrada `HKCU\...\Run` (ligada por padrão) para subir só o agente no login.
- Um agente sem IPC (`Serve` desistiu, IPC-018) encerra em vez de ficar na bandeja, para não segurar o mutex e
  bloquear o relançamento pela UI (EXE-026).

Named pipe, JSON-RPC, a regra "fechar a UI não encerra o agente" e o agente como dono do tempo e das migrações
continuam valendo, como na ADR 0003.

## Alternativas consideradas
- **Dois executáveis (0003):** mais um arquivo para o usuário entender, sem ganho de isolamento.
- **Processo único com "fechar para a bandeja":** um crash do WebView derrubaria o rastreamento.

## Consequências
- ✅ Um só arquivo para distribuir, sem perder o isolamento entre UI e rastreamento.
- ⚠️ O binário do agente carrega o código do Wails (mais megabytes em memória mapeada; o WebView2 não é iniciado).
- ⚠️ O nome `arcadia-agent.exe` citado na ADR 0006 fica obsoleto.
- ⚠️ A preferência de início com o Windows fica na tabela `app_setting` do SQLite, cujo dono é o agente.
