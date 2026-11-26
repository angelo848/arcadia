import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import {
  changeField,
  createFormState,
  editFormState,
  localToUtcIso,
  manualAdjustmentSeconds,
  sessionReasonMessages,
  showIncludedInTotal,
  toCreateInput,
  toUpdatePatch,
  utcIsoToLocal,
  validateSessionForm,
} from './sessionForm'
import type { EditReason, SessionFormValues } from './sessionForm'
import type { SessionItemView } from './library'

// Phase 16 (edicao-sessoes): pure logic of the session form. The fixed zone is Dúvida 9 of tasks.md.
const originalTZ = process.env.TZ
const originalSystemTime = Date.now()

beforeAll(() => {
  process.env.TZ = 'America/Sao_Paulo'
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-11-25T20:00:00Z'))
})

afterAll(() => {
  vi.useRealTimers()
  if (originalTZ === undefined) {
    delete process.env.TZ
  } else {
    process.env.TZ = originalTZ
  }
  void originalSystemTime
})

// T of the contract §5 table. Sao Paulo is UTC-3 (no DST), so 20:00Z is 17:00 local.
const T = new Date('2026-11-25T20:00:00Z')
const at = (offsetMs: number) => new Date(T.getTime() + offsetMs)
const H = 3_600_000
const MIN = 60_000

function values(over: Partial<SessionFormValues> = {}): SessionFormValues {
  return {
    start: '2026-11-25T16:00',
    end: '2026-11-25T17:00',
    hours: '1',
    minutes: '0',
    includedInTotal: false,
    ...over,
  }
}

function fromInstants(start: Date, end: Date, hours: string, minutes: string): SessionFormValues {
  return values({ start: utcIsoToLocal(start.toISOString()), end: utcIsoToLocal(end.toISOString()), hours, minutes })
}

function item(over: Partial<SessionItemView> = {}): SessionItemView {
  return {
    sessionId: 7,
    gameId: 10,
    state: 'ended',
    startedAt: '2026-11-25T14:00:00.000Z',
    endedAt: '2026-11-25T16:00:00.000Z',
    activeSeconds: 5430, // 1 h 30 min 30 s inside a 2 h interval
    endReason: 'exited',
    source: 'tracked',
    editedAt: '',
    ...over,
  }
}

const messages = (e: Record<string, string | undefined>) => Object.values(e).filter(Boolean)
const ctx = (over: Partial<{ now: Date; source: string; adjustmentSeconds: number | null }> = {}) => ({
  now: T,
  source: 'tracked',
  adjustmentSeconds: null,
  ...over,
})

describe('SEL-005 formulário e valores iniciais', () => {
  it('SEL-005/criação: Fim = agora arredondado para baixo ao minuto, Início = Fim − 1 h e Tempo jogado = 1 h 00', () => {
    const st = createFormState(new Date('2026-11-25T20:00:45Z'))
    expect(st.values.end).toBe('2026-11-25T17:00')
    expect(st.values.start).toBe('2026-11-25T16:00')
    expect(st.values.hours).toBe('1')
    expect(st.values.minutes).toBe('0')
    expect(st.values.includedInTotal).toBe(false)
    expect(st.activeFollowsSpan).toBe(true)
    expect(st.initial).toEqual(st.values)
  })

  it('SEL-005/edição: os campos vêm da sessão; o tempo jogado com segundos é mostrado arredondado para baixo ao minuto', () => {
    const st = editFormState(item())
    expect(st.values.start).toBe('2026-11-25T11:00')
    expect(st.values.end).toBe('2026-11-25T13:00')
    expect(st.values.hours).toBe('1')
    expect(st.values.minutes).toBe('30')
    expect(st.initial).toEqual(st.values)
  })
})

describe('SEL-006 Tempo jogado acompanha o intervalo', () => {
  it('SEL-006/com Tempo jogado igual a Fim − Início, mudar Início ou Fim atualiza o Tempo jogado', () => {
    const st = createFormState(T)
    const a = changeField(st, 'start', '2026-11-25T15:00')
    expect([a.values.hours, a.values.minutes]).toEqual(['2', '0'])
    const b = changeField(a, 'end', '2026-11-25T16:30')
    expect([b.values.hours, b.values.minutes]).toEqual(['1', '30'])
  })

  it('SEL-006/depois que o usuário muda o Tempo jogado para outro valor, mudar Início ou Fim não o altera mais', () => {
    const st = changeField(createFormState(T), 'minutes', '30')
    expect(st.activeFollowsSpan).toBe(false)
    const a = changeField(st, 'start', '2026-11-25T15:00')
    expect([a.values.hours, a.values.minutes]).toEqual(['1', '30'])
    const b = changeField(a, 'end', '2026-11-25T18:00')
    expect([b.values.hours, b.values.minutes]).toEqual(['1', '30'])
  })

  it('SEL-006/voltar o Tempo jogado ao valor do intervalo religa o acompanhamento', () => {
    // Behavior fixed here: it re-links only when the typed value equals Fim − Início at the moment of the edit.
    const unlinked = changeField(createFormState(T), 'minutes', '30')
    const relinked = changeField(unlinked, 'minutes', '0')
    expect(relinked.activeFollowsSpan).toBe(true)
    const moved = changeField(relinked, 'start', '2026-11-25T15:00')
    expect([moved.values.hours, moved.values.minutes]).toEqual(['2', '0'])
  })
})

describe('SEL-007 validação antes do envio', () => {
  type Row = {
    n: number
    start: Date
    end: Date
    hours: string
    minutes: string
    source?: string
    reason: EditReason | null
  }
  const rows: Row[] = [
    { n: 1, start: at(-2 * H), end: at(-H), hours: '1', minutes: '0', reason: null },
    { n: 2, start: at(-2 * H), end: at(0), hours: '2', minutes: '0', reason: null },
    { n: 3, start: at(-H), end: at(-H), hours: '0', minutes: '0', reason: 'end-not-after-start' },
    { n: 4, start: at(-H), end: at(MIN), hours: '0', minutes: '30', reason: 'in-future' },
    {
      n: 5,
      start: new Date('1979-12-31T23:59:00Z'),
      end: new Date('1980-01-01T01:00:00Z'),
      hours: '0',
      minutes: '30',
      reason: 'too-early',
    },
    { n: 6, start: at(-25 * H), end: at(0), hours: '2', minutes: '0', reason: 'span-too-long' },
    { n: 7, start: at(-25 * H), end: at(0), hours: '2', minutes: '0', source: 'import', reason: null },
    { n: 8, start: at(-H), end: at(0), hours: '0', minutes: '0', reason: 'active-too-short' },
    { n: 9, start: at(-H), end: at(0), hours: '1', minutes: '1', reason: 'active-exceeds-span' },
    { n: 10, start: at(-24 * H), end: at(0), hours: '24', minutes: '0', reason: null },
  ]

  it.each(rows)('SEL-007/validateSessionForm: linha $n da tabela do contrato §5', (r) => {
    const errors = validateSessionForm(fromInstants(r.start, r.end, r.hours, r.minutes), ctx({ source: r.source ?? 'tracked' }))
    if (r.reason === null) {
      expect(messages(errors)).toEqual([])
    } else {
      expect(messages(errors)).toContain(sessionReasonMessages[r.reason])
    }
  })

  it('SEL-007/sessão importada não tem limite de 24 h (linha 7)', () => {
    const v = fromInstants(at(-300 * H), at(0), '2', '0')
    expect(messages(validateSessionForm(v, ctx({ source: 'import' })))).toEqual([])
    expect(messages(validateSessionForm(v, ctx({ source: 'manual' })))).toContain(sessionReasonMessages['span-too-long'])
  })

  it('SEL-007/Tempo jogado menor que 1 min é active-too-short; maior que o intervalo é active-exceeds-span', () => {
    const short = validateSessionForm(values({ hours: '0', minutes: '0' }), ctx())
    expect(messages(short)).toContain(sessionReasonMessages['active-too-short'])
    const long = validateSessionForm(values({ hours: '1', minutes: '1' }), ctx())
    expect(messages(long)).toContain(sessionReasonMessages['active-exceeds-span'])
  })

  it('SEL-007/minutos fora de 0..59, campo vazio e texto não numérico viram erro do campo com a mensagem proposta', () => {
    // Messages are the proposals of Dúvida 1 of tasks.md (to be confirmed by the spec-writer).
    expect(validateSessionForm(values({ minutes: '60' }), ctx()).active).toBe('Os minutos vão de 0 a 59.')
    expect(validateSessionForm(values({ hours: '' }), ctx()).active).toBe('Preencha este campo.')
    expect(validateSessionForm(values({ minutes: '' }), ctx()).active).toBe('Preencha este campo.')
    expect(validateSessionForm(values({ hours: 'abc' }), ctx()).active).toBe('Informe um número válido.')
    expect(validateSessionForm(values({ start: '' }), ctx()).start).toBe('Preencha este campo.')
    expect(validateSessionForm(values({ end: '' }), ctx()).end).toBe('Preencha este campo.')
  })

  it('SEL-007/as mensagens dos reasons são as de SED-036 (tabela do contrato §4)', () => {
    expect(sessionReasonMessages).toMatchObject({
      'end-not-after-start': 'O fim precisa ser depois do início.',
      'in-future': 'O fim não pode estar no futuro.',
      'too-early': 'O início não pode ser antes de 1980.',
      'span-too-long': 'Uma sessão pode ter no máximo 24 horas.',
      'active-too-short': 'O tempo jogado precisa ter pelo menos 1 minuto.',
      'active-exceeds-span': 'O tempo jogado não pode passar do intervalo entre início e fim.',
      discarded: 'Sessões descartadas não podem ser editadas, só excluídas.',
      'exceeds-adjustment': 'O tempo jogado é maior que a parte do tempo total que não vem de sessões.',
      'empty-patch': 'Nada para salvar.',
    })
  })

  it('SEL-007/o formulário só aceita Tempo jogado ≥ 1 min enquanto o agente aceita ≥ 30 s', () => {
    expect(messages(validateSessionForm(values({ hours: '0', minutes: '1' }), ctx()))).toEqual([])
    expect(messages(validateSessionForm(values({ hours: '0', minutes: '0' }), ctx()))).toContain(
      sessionReasonMessages['active-too-short'],
    )
  })
})

describe('SEL-009 "Já está no tempo total"', () => {
  it('SEL-009/com a caixa marcada, Tempo jogado acima do ajuste é exceeds-adjustment; igual ao ajuste é válido', () => {
    const v = values({ includedInTotal: true, hours: '1', minutes: '0' })
    expect(messages(validateSessionForm(v, ctx({ adjustmentSeconds: 1800 })))).toContain(
      sessionReasonMessages['exceeds-adjustment'],
    )
    expect(messages(validateSessionForm(v, ctx({ adjustmentSeconds: 3600 })))).toEqual([])
    expect(messages(validateSessionForm(v, ctx({ adjustmentSeconds: null })))).toEqual([])
  })

  it('SEL-009/a caixa só é oferecida na criação e com ajuste positivo (showIncludedInTotal)', () => {
    const withAdjustment = { totalPlaySeconds: 7200, sessionPlaySeconds: 3600 }
    expect(showIncludedInTotal('create', withAdjustment)).toBe(true)
    expect(showIncludedInTotal('create', { totalPlaySeconds: 3600, sessionPlaySeconds: 3600 })).toBe(false)
    expect(showIncludedInTotal('create', { totalPlaySeconds: 0, sessionPlaySeconds: 3600 })).toBe(false)
    expect(showIncludedInTotal('edit', withAdjustment)).toBe(false)
    expect(manualAdjustmentSeconds(withAdjustment)).toBe(3600)
  })
  // The help text "X a mais que as sessões" is rendered by the modal: covered by T29 (SessionFormModal.test.tsx).
})

describe('SEL-008 envio só do que mudou', () => {
  it('SEL-008/toCreateInput envia os quatro campos com os instantes em UTC', () => {
    const input = toCreateInput(values({ hours: '1', minutes: '30', end: '2026-11-25T17:30', includedInTotal: true }))
    expect(Object.keys(input).sort()).toEqual(['activeSeconds', 'endedAt', 'includedInTotal', 'startedAt'])
    expect(new Date(input.startedAt).getTime()).toBe(Date.parse('2026-11-25T19:00:00Z'))
    expect(new Date(input.endedAt).getTime()).toBe(Date.parse('2026-11-25T20:30:00Z'))
    expect(input.activeSeconds).toBe(5400)
    expect(input.includedInTotal).toBe(true)
  })

  it('SEL-008/toUpdatePatch envia só os campos alterados; Tempo jogado intocado não é enviado e os segundos da rastreada ficam intactos', () => {
    // Interval 2 h, active 1 h 30 min 30 s: the active time does not follow the interval.
    const st = editFormState(item())
    const end = changeField(st, 'end', '2026-11-25T14:00')
    const patch = toUpdatePatch(st.initial, end.values, 5430)
    expect(Object.keys(patch)).toEqual(['endedAt'])
    expect(new Date(patch.endedAt as string).getTime()).toBe(Date.parse('2026-11-25T17:00:00Z'))

    const active = changeField(st, 'hours', '2')
    const patch2 = toUpdatePatch(st.initial, active.values, 5430)
    expect(patch2).toEqual({ activeSeconds: 2 * 3600 + 30 * 60 })
  })

  it('SEL-008/patch sem nenhuma alteração é vazio (Salvar não envia)', () => {
    const st = editFormState(item())
    expect(toUpdatePatch(st.initial, st.values, 5430)).toEqual({})
  })
})

describe('SEL-016 hora local e horário de verão', () => {
  const zone = 'America/New_York' // Sao Paulo has no DST anymore, so these cases use a zone that has it.
  const withZone = (fn: () => void) => {
    const previous = process.env.TZ
    process.env.TZ = zone
    try {
      fn()
    } finally {
      process.env.TZ = previous
    }
  }

  it('SEL-016/hora local que existe duas vezes (fim do horário de verão) converte para o instante que o Date do navegador escolhe e o valor devolvido ao campo é o convertido', () => {
    withZone(() => {
      const local = '2026-11-01T01:30' // happens in EDT and again in EST
      const iso = localToUtcIso(local)
      expect(iso).not.toBeNull()
      expect(new Date(iso as string).getTime()).toBe(new Date(local).getTime())
      expect([Date.parse('2026-11-01T05:30:00Z'), Date.parse('2026-11-01T06:30:00Z')]).toContain(new Date(iso as string).getTime())
      expect(utcIsoToLocal(iso as string)).toBe(local)
    })
  })

  it('SEL-016/hora local que não existe (início do horário de verão) é ajustada pelo Date e o campo mostra o valor ajustado', () => {
    withZone(() => {
      const local = '2026-03-08T02:30' // skipped: clocks go from 02:00 to 03:00
      const iso = localToUtcIso(local)
      expect(iso).not.toBeNull()
      expect(new Date(iso as string).getTime()).toBe(new Date(local).getTime())
      const shown = utcIsoToLocal(iso as string)
      expect(shown).not.toBe(local)
      expect(localToUtcIso(shown)).toBe(iso)
    })
  })

  it('SEL-016/ida e volta localToUtcIso e utcIsoToLocal para um instante comum', () => {
    expect(utcIsoToLocal('2026-11-25T20:00:00.000Z')).toBe('2026-11-25T17:00')
    const iso = localToUtcIso('2026-11-25T17:00')
    expect(new Date(iso as string).getTime()).toBe(Date.parse('2026-11-25T20:00:00Z'))
    expect(utcIsoToLocal(iso as string)).toBe('2026-11-25T17:00')
    expect(localToUtcIso('')).toBeNull()
  })
})
