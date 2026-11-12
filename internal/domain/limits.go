package domain

// Limits shared by the domain validators and, through the IPC contract
// (x-limits), by the UI. Values come from design.md, "Constantes (domain)".
const (
	MaxTitleRunes       = 200   // GAM-003, existing
	MaxNameRunes        = 100   // DOM-015, DOM-016
	MaxDescriptionRunes = 10000 // EDT-001
	MaxFilterTextRunes  = 200   // FLT-001
	MaxFilterIDs        = 50    // FLT-001
	MaxNameList         = 50    // IPC: NameList maxItems
	MaxFilterStatuses   = 5     // FLT-005
	YearMin             = 1900  // DOM-013
	YearMax             = 2100  // DOM-013
	DefaultPageLimit    = 200   // LIB-002
	MaxPageLimit        = 500   // LIB-002
	DefaultSessionLimit = 50    // STO-027
	MaxSessionLimit     = 200   // STO-027
)
