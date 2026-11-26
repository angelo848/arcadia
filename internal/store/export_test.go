package store

// MaxTotalPlaySecondsForTest exposes the store's local ceiling of a game's total so a black-box test
// can compare it with ipc.MaxPlaySeconds (SED-013, tasks T08/T16).
const MaxTotalPlaySecondsForTest = maxTotalPlaySeconds
