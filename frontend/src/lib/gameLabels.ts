import type { GameStatus } from './filters'

export { playModeLabels } from './library'

// statusLabels names each game status in the UI (TDK-008).
export const statusLabels: Record<GameStatus, string> = {
  not_played: 'Não jogado',
  playing: 'Jogando',
  completed: 'Concluído',
  abandoned: 'Abandonado',
  backlog: 'Na fila',
}
