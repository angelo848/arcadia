# 0024 — Remoção do início automático com o Windows

- **Status:** proposto (aguarda aprovação do usuário junto com a spec [phase-5-1-ajustes](../specs/phase-5-1-ajustes/requirements.md)).
  Substitui a parte de início com o Windows da [ADR 0014](0014-executavel-unico-dois-processos.md) e a área AST-002..AST-011 de [single-exe](../specs/single-exe/requirements.md).
- **Data:** 2026-11-12

## Contexto
O agente subia com o Windows por uma entrada `HKCU\...\Run` (ADR 0014, AST-002..011). Com a decisão de rodar sempre como
administrador ([ADR 0023](0023-execucao-como-administrador.md)), o início automático ficaria em conflito: um agente sem
elevação subiria no login e atrapalharia a abertura elevada da UI, que precisaria encerrá-lo antes. O usuário pediu a
remoção do recurso.

## Decisão
- Não há mais entrada `Run`, item "Iniciar com o Windows" na bandeja, nem leitura ou escrita da preferência `autostart`.
- Na primeira subida de uma versão com esta decisão, a entrada `Arcadia` já existente em `HKCU\...\Run` é apagada, se
  apontar para o mesmo executável (ADM-004 da spec 5.1).
- A linha `autostart` no `app_setting` fica sem uso, sem migração nova.
- O código de autostart (`internal/agent/autostart.go`, `internal/winapi/autostart_windows.go`) é removido.

## Alternativas consideradas
- **Manter o recurso desligado por padrão:** a opção continuaria na bandeja e o conflito com a UI elevada permaneceria.
  Descartada.
- **Manter o recurso e iniciar o agente elevado por tarefa agendada:** exige privilégio para criar a tarefa e outro
  mecanismo de inicialização. Descartada.

## Consequências
- ✅ Um só agente por sessão do Windows, sempre elevado, aberto pelo usuário.
- ✅ Menos código de registro e menos testes de autostart.
- ⚠️ O tempo de jogo só é contado quando o Arcadia está aberto. Quem jogava sem abrir o Arcadia e contava com a subida no
  login deixa de ter o tempo registrado. Aceito pelo usuário.
- ⚠️ Usuários que já tinham o início ligado precisam que a primeira subida apague a entrada; por isso o requisito ADM-004.
