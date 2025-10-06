package storage

import "time"

// Entry represents a single normalized scope entry for a program.
type Entry struct {
	// Program info
	ProgramURL string
	Platform   string
	Handle     string

	// Target info
	TargetNormalized string
	TargetRaw        string
	Category         string
	Description      string
	InScope          bool
	IsHistorical     bool
}

// Change captures a single change event for auditing or printing.
type Change struct {
	OccurredAt time.Time

	// Program info
	ProgramURL string
	Platform   string
	Handle     string

	// Target info
	TargetNormalized string
	Category         string
	InScope          bool
	ChangeType       string // added | updated | removed
}

// TargetItem is a light wrapper for building entries.
type TargetItem struct {
	URI         string
	Category    string
	Description string
	InScope     bool
}
