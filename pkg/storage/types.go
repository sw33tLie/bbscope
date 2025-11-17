package storage

import "time"

// Entry represents a single normalized scope entry for a program.
type Entry struct {
	// Program info
	ProgramURL string
	Platform   string
	Handle     string

	// Display target info (variant or raw)
	TargetNormalized string
	TargetRaw        string

	// Base target info (always deterministic/raw)
	BaseTargetNormalized string
	BaseTargetRaw        string

	Category     string
	Description  string
	InScope      bool
	IsBBP        bool
	IsHistorical bool
	Source       string
}

// Change captures a single change event for auditing or printing.
type Change struct {
	OccurredAt         time.Time
	ProgramURL         string
	Platform           string
	Handle             string
	TargetNormalized   string
	TargetRaw          string
	TargetAINormalized string
	Category           string
	InScope            bool
	IsBBP              bool
	ChangeType         string
}

// UpsertEntry represents the raw scope item along with its variants.
type UpsertEntry struct {
	ProgramURL       string
	Platform         string
	Handle           string
	TargetNormalized string
	TargetRaw        string
	Category         string
	Description      string
	InScope          bool
	IsBBP            bool
	Variants         []EntryVariant
}

// EntryVariant represents a derived/expanded target tied to a raw entry.
type EntryVariant struct {
	AINormalized string
	InScope      bool
}

// TargetItem is a light wrapper for building entries.
type TargetItem struct {
	URI         string
	Category    string
	Description string
	InScope     bool
	IsBBP       bool
	Variants    []TargetVariant
}

// TargetVariant captures a requested expansion for a target.
type TargetVariant struct {
	Value      string
	HasInScope bool
	InScope    bool
}
