# 0025 — Rastreio por processo adotado (jogos que saem da árvore do Arcadia)

- **Status:** aceito (aprovado com a fase 6, [phase-6-importers](../specs/phase-6-importers/requirements.md)). A adoção mede saída por snapshot, não por SYNCHRONIZE: ver design §5.
- **Data:** 2026-11-12

## Contexto
Steam, Epic e outras lojas abrem o jogo como filho do cliente, fora do Job Object do Arcadia (ARCHITECTURE §4.2, camada 2).
O agente precisa reconhecer o processo do jogo e acompanhá-lo sem o Job.

## Decisão
- O processo casado por regra (RUL-001, RUL-002) é adotado pela sessão e entra no tracker como um `Job` de
  implementação própria (`adoptedJob`), sem Job Object: os `Events` vêm de `WaitForSingleObject` sobre um handle
  `SYNCHRONIZE`. A máquina de estados, a pausa e a carência não mudam.
- O watcher faz um snapshot de processos a cada ~1,5 s, dentro de uma goroutine com dono e saída (CLAUDE.md).
- A sessão adotada recebe `tracking = process`; a página do jogo informa isso (RUL-005).
- Sem Job, filhos fora das regras não contam: limitação aceita e visível ao usuário.

## Alternativas consideradas
- **Job Object sobre o cliente da loja:** o cliente já está fora do Job do Arcadia; não há como associar o processo depois.
- **ETW (`Microsoft-Windows-Kernel-Process`):** eventos instantâneos, mas exigem privilégio de administrador e uma
  sessão de rastreio à parte. Fica como upgrade (ARCHITECTURE §4.2).
- **Heurística de janela em tela cheia com pergunta ao usuário:** adiada (RUL-006).

## Consequências
- ✅ Reaproveita a máquina de estados e a pausa existentes.
- ⚠️ Filhos e processos fora das regras não contam nesse modo.
- ⚠️ Um instalador dentro da pasta de instalação pode ser adotado; a carência encerra quando ele sai.
