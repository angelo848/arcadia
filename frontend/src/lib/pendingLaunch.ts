// PendingLaunch is the launch whose call is in flight: the game and the mode it
// was started with [UI-021]. Both buttons of that game stay disabled until it ends.
export type PendingLaunch = { id: number; mode: 'tracked' | 'untracked' } | null
