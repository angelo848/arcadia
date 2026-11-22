import { describe, expect, it } from 'vitest'

// Scans the production sources of frontend/src/bigbox/ (BBX-003, BBX-025, BBX-029).
const raw = import.meta.glob('./**/*.{ts,tsx}', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const files = Object.entries(raw).filter(([p]) => !/\.test\.tsx?$/.test(p))

function lineOf(src: string, index: number): number {
  return src.slice(0, index).split('\n').length
}

type Import = { path: string; line: number; names: string; from: string }

// Every `import ... from '...'` and bare `import '...'` of the file (type-only imports included).
function importsOf(path: string, src: string): Import[] {
  const out: Import[] = []
  for (const m of src.matchAll(/import\s+(?:type\s+)?([^'";]*?)\s*from\s*['"]([^'"]+)['"]|import\s*['"]([^'"]+)['"]/g)) {
    out.push({ path, line: lineOf(src, m.index ?? 0), names: m[1] ?? '', from: m[2] ?? m[3] ?? '' })
  }
  return out
}

const imports = files.flatMap(([p, s]) => importsOf(p, s))

const palette =
  /\b(bg|text|border|ring|outline|from|via|to|fill|stroke|divide|placeholder|accent|caret|decoration|shadow)-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white)(-\d+)?\b/g
const literalColor = /#[0-9a-fA-F]{3,8}\b|\brgba?\(|\bhsla?\(/g

function classNames(src: string): { text: string; index: number }[] {
  const out: { text: string; index: number }[] = []
  const re = /className\s*=\s*(?:"([^"]*)"|'([^']*)'|\{`([^`]*)`\}|\{\s*"([^"]*)"\s*\}|\{\s*'([^']*)'\s*\})/g
  for (const m of src.matchAll(re)) {
    out.push({ text: m[1] ?? m[2] ?? m[3] ?? m[4] ?? m[5] ?? '', index: m.index ?? 0 })
  }
  return out
}

describe('varredura do Big Box', () => {
  it('sanidade: a varredura encontra as fontes de produção de bigbox/', () => {
    expect(files.length).toBeGreaterThan(10)
    expect(files.some(([p]) => p === './BigBox.tsx')).toBe(true)
    expect(files.some(([p]) => p.endsWith('.test.ts') || p.endsWith('.test.tsx'))).toBe(false)
    expect(imports.length).toBeGreaterThan(10)
  })

  it('BBX-003/varredura de frontend/src/bigbox/: nenhum import de componentes de estatísticas, emuladores, plataformas, provedores, importação, edição, remoção, metadados nem de bindings de escrita além de LaunchGame, LaunchGameUntracked e BringToFront', () => {
    const bad: string[] = []
    const forbiddenModule =
      /(\/components\/|StatsPage|Emulator|Platform(?!Id)|Provider|Import|Edit|Remove|Delete|MetadataDialog|MetadataApply|wailsjs\/go\/main\/App)/
    for (const i of imports) {
      // Sibling files of bigbox/ and the read-only lib modules are fine; anything from components/ is not.
      if (i.from.startsWith('.') && !i.from.includes('/components/') && !i.from.startsWith('../lib/') && !forbiddenModule.test(i.from)) {
        continue
      }
      if (i.from.startsWith('../lib/')) {
        // lib modules are allowed, but not the write modules of other areas.
        if (/\/lib\/(stats|emulators|platforms|providers|imports|importers|edit)/i.test(i.from)) {
          bad.push(`${i.path}:${i.line}: ${i.from}`)
        }
        continue
      }
      if (i.from.startsWith('.') || /wailsjs/.test(i.from)) bad.push(`${i.path}:${i.line}: ${i.from}`)
    }
    // Imported names that are write bindings or write helpers.
    const writeName =
      /\b(PauseSession|ResumeSession|ToggleSession|UpdateGame|DeleteGame|AddPlatform|Platform(Delete|SetEmulator|SetIcon|Update)|Emulator(Add|Delete|Update)|ProviderSetKey|ProviderDeleteKey|ImportApply|ScrapeApply|MediaQueue|MediaCancel|MetadataApply|MetadataBatch\w*|pauseSession|resumeSession|updateGame|deleteGame|importApply|applyMetadata)\b/
    for (const i of imports) {
      if (writeName.test(i.names)) bad.push(`${i.path}:${i.line}: nomes ${i.names}`)
    }
    expect(bad, `imports proibidos:\n${bad.join('\n')}`).toEqual([])
  })

  it('BBX-003/o código de bigbox/ não chama window.go diretamente nem binding de escrita fora de launchGame, launchGameUntracked e bringToFront', () => {
    const hits: string[] = []
    const re = /\b(window\.go|\.go\.main|PauseSession|ResumeSession|ToggleSession|UpdateGame|DeleteGame|ImportApply|ScrapeApply|MetadataApply|MediaQueue|MediaCancel|ProviderSetKey|ProviderDeleteKey)\b/g
    for (const [p, src] of files) {
      for (const m of src.matchAll(re)) hits.push(`${p}:${lineOf(src, m.index ?? 0)}: ${m[0]}`)
    }
    expect(hits, `chamadas proibidas:\n${hits.join('\n')}`).toEqual([])
  })

  it('BBX-003/varredura: bigbox/ não importa lib/timeToBeatEdit nem usa TimeToBeatRefresh, TimeToBeatSearch, TimeToBeatChoose ou TimeToBeatSetSettings (nem as funções de escrita)', () => {
    const bad: string[] = []
    for (const i of imports) {
      if (/timeToBeatEdit/i.test(i.from)) bad.push(`${i.path}:${i.line}: import de ${i.from}`)
    }
    const writes =
      /\b(TimeToBeatRefresh|TimeToBeatSearch|TimeToBeatChoose|TimeToBeatSetSettings|refreshTimeToBeat|searchTimeToBeat|chooseTimeToBeat|setTimeToBeatSettings|useTimeToBeatSettings)\b/g
    for (const [p, src] of files) {
      for (const m of src.matchAll(writes)) bad.push(`${p}:${lineOf(src, m.index ?? 0)}: ${m[0]}`)
    }
    expect(bad, `escrita do tempo para zerar em bigbox/:\n${bad.join('\n')}`).toEqual([])
  })

  it('BBX-059/varredura: o único acesso ao tempo para zerar em bigbox/ é a leitura de lib/timeToBeat (nenhum binding TimeToBeat* direto)', () => {
    const bad: string[] = []
    for (const i of imports) {
      if (/timeToBeat/i.test(i.from) && !/^(\.\.\/)+lib\/timeToBeat$/.test(i.from)) bad.push(`${i.path}:${i.line}: import de ${i.from}`)
    }
    // Bindings are named TimeToBeat<Verb>; bigbox/ reaches TimeToBeatGet only through useTimeToBeat of lib/.
    const binding = /\b(TimeToBeat(Get|GetSettings|Refresh|Search|Choose|SetSettings)|go\.main\.App\.TimeToBeat\w*)\b/g
    for (const [p, src] of files) {
      for (const m of src.matchAll(binding)) bad.push(`${p}:${lineOf(src, m.index ?? 0)}: ${m[0]}`)
    }
    expect(bad, `acesso ao tempo para zerar fora do módulo de leitura:\n${bad.join('\n')}`).toEqual([])
  })

  it('BBX-059/varredura: a leitura com fetch fica no shell e nos temas só vem pelo modelo (nenhum tema chama useTimeToBeat)', () => {
    const hits = files
      .filter(([p]) => p.startsWith('./themes/'))
      .flatMap(([p, src]) => [...src.matchAll(/\buseTimeToBeat\w*\b|fetchAfterMs/g)].map((m) => `${p}:${lineOf(src, m.index ?? 0)}: ${m[0]}`))
    expect(hits, `temas lendo o tempo para zerar por conta própria:\n${hits.join('\n')}`).toEqual([])
  })

  it('BBX-047/varredura: os temas não importam bindings, eventos nem lib/ de escrita (só o modelo e as ações)', () => {
    const bad: string[] = []
    for (const i of imports.filter((x) => x.path.startsWith('./themes/'))) {
      if (/\/lib\/(?!timeToBeat$)/.test(i.from)) bad.push(`${i.path}:${i.line}: ${i.from}`)
      if (/wailsjs|\/useBigBoxEvents|\/useBigBoxData|\/useFocusedData/.test(i.from)) bad.push(`${i.path}:${i.line}: ${i.from}`)
    }
    expect(bad, `imports proibidos nos temas:\n${bad.join('\n')}`).toEqual([])
  })

  it('BBX-025/a varredura de UIX-006 cobre frontend/src/bigbox/ e não acha cor literal nem classe de paleta', () => {
    const tsx = files.filter(([p]) => p.endsWith('.tsx'))
    expect(tsx.length, 'há .tsx em bigbox/').toBeGreaterThan(0)
    const hits: string[] = []
    for (const [path, src] of tsx) {
      for (const m of src.matchAll(palette)) hits.push(`${path}:${lineOf(src, m.index ?? 0)}: ${m[0]}`)
      const regions: { text: string; index: number }[] = [...classNames(src)]
      for (const m of src.matchAll(/style\s*=\s*\{\{[\s\S]*?\}\}/g)) regions.push({ text: m[0], index: m.index ?? 0 })
      for (const r of regions) {
        for (const c of r.text.matchAll(literalColor)) hits.push(`${path}:${lineOf(src, r.index)}: ${c[0]}`)
      }
    }
    expect(hits, `cores literais ou paleta:\n${hits.join('\n')}`).toEqual([])
  })

  it('BBX-025/Panel.tsx, capacity.ts e useViewport.ts entram na varredura e não têm cor literal nem classe de paleta', () => {
    for (const f of ['./Panel.tsx', './themes/faixa/capacity.ts', './useViewport.ts']) {
      const hit = files.find(([p]) => p === f)
      expect(hit, `${f} entra na varredura`).toBeDefined()
      const src = hit![1]
      const bad = [...src.matchAll(palette), ...src.matchAll(literalColor)].map((m) => `${f}:${lineOf(src, m.index ?? 0)}: ${m[0]}`)
      expect(bad, `cores literais ou paleta:\n${bad.join('\n')}`).toEqual([])
    }
  })

  it('BBX-037/nenhum arquivo de bigbox/ pinta fundo de texto com --arc-bb-scrim (só o preenchimento de BackgroundStage o usa)', () => {
    const hits: string[] = []
    for (const [p, src] of files) {
      if (p === './BackgroundStage.tsx') continue
      for (const m of src.matchAll(/--arc-bb-scrim|\bbg-bb-scrim\b|\bbb-scrim\b/g)) hits.push(`${p}:${lineOf(src, m.index ?? 0)}: ${m[0]}`)
    }
    expect(hits, `scrim fora do fundo:\n${hits.join('\n')}`).toEqual([])
  })

  it('BBX-029/os arquivos de bigbox/ não importam nenhum binding Video*, MediaQueue nem MetadataApply', () => {
    const bad: string[] = []
    const name = /\b(Video\w*|video(Queue|QueueMissing|CancelPending|Status)\w*|MediaQueue|MetadataApply|queueMedia|applyMetadata)\b/
    for (const i of imports) {
      if (name.test(i.names) || /(^|\/)lib\/video(\.|$)|\/video$/.test(i.from)) bad.push(`${i.path}:${i.line}: ${i.names} from ${i.from}`)
    }
    for (const [p, src] of files) {
      for (const m of src.matchAll(/\b(App\.)?(Video(Queue|QueueMissing|CancelPending|Status)|MediaQueue|MetadataApply)\b/g)) {
        bad.push(`${p}:${lineOf(src, m.index ?? 0)}: ${m[0]}`)
      }
    }
    expect(bad, `imports de vídeo ou escrita:\n${bad.join('\n')}`).toEqual([])
  })
})
