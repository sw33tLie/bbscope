package reports

// Report holds the full detail of a single HackerOne report.
type Report struct {
	ID                       string
	Title                    string
	State                    string
	Substate                 string
	CreatedAt                string
	TriagedAt                string
	ClosedAt                 string
	DisclosedAt              string
	VulnerabilityInformation string
	Impact                   string
	CVEIDs                   []string
	ProgramHandle            string
	SeverityRating           string
	CVSSScore                string
	WeaknessName             string
	WeaknessCWE              string
	AssetIdentifier          string
	BountyAmount             string
}

// ReportSummary is the lightweight version returned by the list endpoint.
type ReportSummary struct {
	ID             string
	Title          string
	State          string
	Substate       string
	ProgramHandle  string
	SeverityRating string
	CreatedAt      string
}

// FetchOptions controls which reports to fetch and how to save them.
type FetchOptions struct {
	Programs   []string
	States     []string
	Severities []string
	DryRun     bool
	Overwrite  bool
	OutputDir  string
}
