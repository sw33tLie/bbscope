package reports

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)
var multiUnderscore = regexp.MustCompile(`_+`)

const reportTemplate = `# {{.Title}}

| Field | Value |
|-------|-------|
| **Report ID** | {{.ID}} |
| **Program** | {{.ProgramHandle}} |
| **State** | {{.Substate}} |
{{- if .SeverityRating}}
| **Severity** | {{.SeverityRating}}{{if .CVSSScore}} (CVSS: {{.CVSSScore}}){{end}} |
{{- end}}
{{- if .WeaknessName}}
| **Weakness** | {{.WeaknessName}}{{if .WeaknessCWE}} ({{.WeaknessCWE}}){{end}} |
{{- end}}
{{- if .AssetIdentifier}}
| **Asset** | {{.AssetIdentifier}} |
{{- end}}
{{- if .BountyAmount}}
| **Bounty** | ${{.BountyAmount}} |
{{- end}}
{{- if .CVEList}}
| **CVE(s)** | {{.CVEList}} |
{{- end}}
| **Created** | {{.CreatedAt}} |
{{- if .TriagedAt}}
| **Triaged** | {{.TriagedAt}} |
{{- end}}
{{- if .ClosedAt}}
| **Closed** | {{.ClosedAt}} |
{{- end}}
{{- if .DisclosedAt}}
| **Disclosed** | {{.DisclosedAt}} |
{{- end}}

---
{{if .VulnerabilityInformation}}

## Vulnerability Information

{{.VulnerabilityInformation}}

---
{{end}}
{{- if .Impact}}

## Impact

{{.Impact}}
{{end}}`

var tmpl = template.Must(template.New("report").Parse(reportTemplate))

// templateData wraps Report with computed fields for the template.
type templateData struct {
	Report
	CVEList string
}

func sanitizeFilename(s string) string {
	s = unsafeChars.ReplaceAllString(s, "_")
	s = multiUnderscore.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}

// ReportFilePath returns the filesystem path for a report markdown file.
func ReportFilePath(outputDir string, r *Report) string {
	handle := sanitizeFilename(r.ProgramHandle)
	if handle == "" {
		handle = "unknown"
	}
	filename := fmt.Sprintf("%s_%s.md", r.ID, sanitizeFilename(r.Title))
	return filepath.Join(outputDir, "h1", handle, filename)
}

// WriteReport renders a report as Markdown and writes it to disk.
// Returns true if the file was written, false if skipped.
func WriteReport(r *Report, outputDir string, overwrite bool) (bool, error) {
	path := ReportFilePath(outputDir, r)

	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return false, nil // file exists, skip
		}
	}

	data := templateData{
		Report:  *r,
		CVEList: strings.Join(r.CVEIDs, ", "),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return false, fmt.Errorf("rendering template for report %s: %w", r.ID, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("creating directory for report %s: %w", r.ID, err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return false, fmt.Errorf("writing report %s: %w", r.ID, err)
	}

	return true, nil
}
