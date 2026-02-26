package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// RemoveCustomTarget removes a custom target from the database.
func (d *DB) RemoveCustomTarget(ctx context.Context, target, category, programURL string) error {
	query := `
		DELETE FROM targets_raw
		WHERE target = $1 AND category = $2 AND program_id IN (
			SELECT id FROM programs WHERE url = $3
		)
	`
	res, err := d.sql.ExecContext(ctx, query, target, category, programURL)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("target not found")
	}
	return nil
}

// Program represents a bug bounty program.
type Program struct {
	ID        int64  `json:"id"`
	Platform  string `json:"platform"`
	Handle    string `json:"handle"`
	URL       string `json:"url"`
	IsIgnored bool   `json:"is_ignored"`
	Disabled  bool   `json:"disabled"`
}

// ListPrograms returns all programs.
func (d *DB) ListPrograms(ctx context.Context) ([]Program, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT id, platform, handle, url, is_ignored, disabled FROM programs ORDER BY platform, handle")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var programs []Program
	for rows.Next() {
		var p Program
		var ignored, disabled int
		if err := rows.Scan(&p.ID, &p.Platform, &p.Handle, &p.URL, &ignored, &disabled); err != nil {
			return nil, err
		}
		p.IsIgnored = ignored == 1
		p.Disabled = disabled == 1
		programs = append(programs, p)
	}
	return programs, rows.Err()
}

// ProgramListOptions controls filtering/sorting/pagination for program listing.
type ProgramListOptions struct {
	Platforms   []string // filter by platforms (empty = all)
	Search      string   // search across handle, url, and target names
	SortBy      string   // "handle", "platform", "in_scope_count", "out_of_scope_count"
	SortOrder   string   // "asc" or "desc"
	Page        int
	PerPage     int
	ProgramType string // "bbp", "vdp", or "" (all)
}

// ProgramListEntry is a program with aggregated target counts.
type ProgramListEntry struct {
	Platform        string   `json:"platform"`
	Handle          string   `json:"handle"`
	URL             string   `json:"url"`
	InScopeCount    int      `json:"in_scope_count"`
	OutOfScopeCount int      `json:"out_of_scope_count"`
	IsBBP           bool     `json:"is_bbp"`
	Targets         []string `json:"targets,omitempty"` // target display names for search
}

// ProgramListResult holds paginated results.
type ProgramListResult struct {
	Programs   []ProgramListEntry
	TotalCount int
	Page       int
	PerPage    int
	TotalPages int
}

// ProgramTarget represents a single target within a program detail view.
type ProgramTarget struct {
	TargetDisplay string `json:"target"`     // AI-normalized if available, else raw
	TargetRaw     string `json:"target_raw"`
	Category      string `json:"category"`
	Description   string `json:"description"`
	InScope       bool   `json:"in_scope"`
	IsBBP         bool   `json:"is_bbp"`
}

// ProgramSlug holds the platform and handle for URL generation.
type ProgramSlug struct {
	Platform string
	Handle   string
}

// ListAllProgramsFlat returns all active, non-ignored programs with aggregated target counts
// and target display names. When rawMode is true, AI enhancements are ignored and only raw
// target data is used for names, counts, and scope status.
func (d *DB) ListAllProgramsFlat(ctx context.Context, rawMode bool) ([]ProgramListEntry, error) {
	var query string
	if rawMode {
		query = `
			SELECT p.id, p.platform, p.handle, p.url,
				COALESCE(SUM(CASE WHEN t.in_scope = 1 THEN 1 ELSE 0 END), 0) AS in_scope_count,
				COALESCE(SUM(CASE WHEN t.in_scope = 0 THEN 1 ELSE 0 END), 0) AS out_of_scope_count,
				COALESCE(MAX(t.is_bbp), 0) AS has_bbp
			FROM programs p
			LEFT JOIN targets_raw t ON t.program_id = p.id
			WHERE p.disabled = 0 AND p.is_ignored = 0
			GROUP BY p.id, p.platform, p.handle, p.url
			ORDER BY LOWER(p.handle) ASC
		`
	} else {
		query = `
			SELECT p.id, p.platform, p.handle, p.url,
				COALESCE(SUM(CASE WHEN COALESCE(a.in_scope, t.in_scope) = 1 THEN 1 ELSE 0 END), 0) AS in_scope_count,
				COALESCE(SUM(CASE WHEN COALESCE(a.in_scope, t.in_scope) = 0 THEN 1 ELSE 0 END), 0) AS out_of_scope_count,
				COALESCE(MAX(t.is_bbp), 0) AS has_bbp
			FROM programs p
			LEFT JOIN targets_raw t ON t.program_id = p.id
			LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
			WHERE p.disabled = 0 AND p.is_ignored = 0
			GROUP BY p.id, p.platform, p.handle, p.url
			ORDER BY LOWER(p.handle) ASC
		`
	}

	rows, err := d.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing all programs flat: %w", err)
	}
	defer rows.Close()

	var programs []ProgramListEntry
	idToIdx := make(map[int64]int) // program DB id -> index in programs slice
	for rows.Next() {
		var p ProgramListEntry
		var id int64
		var hasBBP int
		if err := rows.Scan(&id, &p.Platform, &p.Handle, &p.URL, &p.InScopeCount, &p.OutOfScopeCount, &hasBBP); err != nil {
			return nil, err
		}
		p.IsBBP = hasBBP == 1
		idToIdx[id] = len(programs)
		programs = append(programs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch all target display names for active programs
	var targetQuery string
	if rawMode {
		targetQuery = `
			SELECT t.program_id, t.target AS target_display
			FROM targets_raw t
			JOIN programs p ON t.program_id = p.id
			WHERE p.disabled = 0 AND p.is_ignored = 0
			ORDER BY t.program_id
		`
	} else {
		targetQuery = `
			SELECT t.program_id,
				COALESCE(NULLIF(a.target_ai_normalized, ''), t.target) AS target_display
			FROM targets_raw t
			JOIN programs p ON t.program_id = p.id
			LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
			WHERE p.disabled = 0 AND p.is_ignored = 0
			ORDER BY t.program_id
		`
	}
	tRows, err := d.sql.QueryContext(ctx, targetQuery)
	if err != nil {
		return nil, fmt.Errorf("listing target names: %w", err)
	}
	defer tRows.Close()

	for tRows.Next() {
		var programID int64
		var targetDisplay string
		if err := tRows.Scan(&programID, &targetDisplay); err != nil {
			return nil, err
		}
		if idx, ok := idToIdx[programID]; ok {
			programs[idx].Targets = append(programs[idx].Targets, targetDisplay)
		}
	}
	if err := tRows.Err(); err != nil {
		return nil, err
	}

	return programs, nil
}

// ListProgramsPaginated returns a paginated list of programs with aggregated target counts.
// All filtering, sorting, and pagination is done at the SQL level.
func (d *DB) ListProgramsPaginated(ctx context.Context, opts ProgramListOptions) (*ProgramListResult, error) {
	where := "WHERE p.disabled = 0 AND p.is_ignored = 0"
	args := []any{}
	argIdx := 1

	if len(opts.Platforms) > 0 {
		placeholders := make([]string, len(opts.Platforms))
		for i, plat := range opts.Platforms {
			placeholders[i] = fmt.Sprintf("LOWER($%d)", argIdx)
			args = append(args, plat)
			argIdx++
		}
		where += fmt.Sprintf(" AND LOWER(p.platform) IN (%s)", strings.Join(placeholders, ","))
	}

	if opts.Search != "" {
		searchPattern := "%" + opts.Search + "%"
		where += fmt.Sprintf(` AND (p.handle ILIKE $%d OR p.url ILIKE $%d OR EXISTS (
			SELECT 1 FROM targets_raw t2
			LEFT JOIN targets_ai_enhanced a2 ON a2.target_id = t2.id
			WHERE t2.program_id = p.id
			AND (COALESCE(NULLIF(a2.target_ai_normalized, ''), t2.target) ILIKE $%d)
		))`, argIdx, argIdx+1, argIdx+2)
		args = append(args, searchPattern, searchPattern, searchPattern)
		argIdx += 3
	}

	// Build HAVING clause for program type filter
	havingClause := ""
	switch opts.ProgramType {
	case "bbp":
		havingClause = " HAVING COALESCE(MAX(t.is_bbp), 0) = 1"
	case "vdp":
		havingClause = " HAVING COALESCE(MAX(t.is_bbp), 0) = 0"
	}

	// Count query
	var countQuery string
	if havingClause != "" {
		// Need subquery with join + HAVING to filter by program type
		countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM (
			SELECT p.id FROM programs p
			LEFT JOIN targets_raw t ON t.program_id = p.id
			%s
			GROUP BY p.id
			%s
		) sub`, where, havingClause)
	} else {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM programs p %s", where)
	}
	var totalCount int
	if err := d.sql.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("counting programs: %w", err)
	}

	if opts.PerPage <= 0 {
		opts.PerPage = 50
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}

	totalPages := (totalCount + opts.PerPage - 1) / opts.PerPage
	if totalPages < 1 {
		totalPages = 1
	}

	// Sort column mapping
	sortColumn := "LOWER(p.handle)"
	switch opts.SortBy {
	case "handle":
		sortColumn = "LOWER(p.handle)"
	case "platform":
		sortColumn = "LOWER(p.platform)"
	case "in_scope_count":
		sortColumn = "in_scope_count"
	case "out_of_scope_count":
		sortColumn = "out_of_scope_count"
	case "url":
		sortColumn = "LOWER(p.url)"
	}

	sortDir := "ASC"
	if strings.ToLower(opts.SortOrder) == "desc" {
		sortDir = "DESC"
	}

	offset := (opts.Page - 1) * opts.PerPage

	mainQuery := fmt.Sprintf(`
		SELECT p.platform, p.handle, p.url,
			COALESCE(SUM(CASE WHEN COALESCE(a.in_scope, t.in_scope) = 1 THEN 1 ELSE 0 END), 0) AS in_scope_count,
			COALESCE(SUM(CASE WHEN COALESCE(a.in_scope, t.in_scope) = 0 THEN 1 ELSE 0 END), 0) AS out_of_scope_count,
			COALESCE(MAX(t.is_bbp), 0) AS has_bbp
		FROM programs p
		LEFT JOIN targets_raw t ON t.program_id = p.id
		LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
		%s
		GROUP BY p.id, p.platform, p.handle, p.url
		%s
		ORDER BY %s %s, LOWER(p.handle) ASC
		LIMIT $%d OFFSET $%d
	`, where, havingClause, sortColumn, sortDir, argIdx, argIdx+1)

	args = append(args, opts.PerPage, offset)

	rows, err := d.sql.QueryContext(ctx, mainQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("listing programs: %w", err)
	}
	defer rows.Close()

	var programs []ProgramListEntry
	for rows.Next() {
		var p ProgramListEntry
		var hasBBP int
		if err := rows.Scan(&p.Platform, &p.Handle, &p.URL, &p.InScopeCount, &p.OutOfScopeCount, &hasBBP); err != nil {
			return nil, err
		}
		p.IsBBP = hasBBP == 1
		programs = append(programs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ProgramListResult{
		Programs:   programs,
		TotalCount: totalCount,
		Page:       opts.Page,
		PerPage:    opts.PerPage,
		TotalPages: totalPages,
	}, nil
}

// GetProgramByPlatformHandle fetches a single active program by platform and handle.
func (d *DB) GetProgramByPlatformHandle(ctx context.Context, platform, handle string) (*Program, error) {
	query := `SELECT id, platform, handle, url, is_ignored, disabled
		FROM programs
		WHERE LOWER(platform) = LOWER($1) AND LOWER(handle) = LOWER($2)
		AND disabled = 0 AND is_ignored = 0
		LIMIT 1`

	var p Program
	var ignored, disabled int
	err := d.sql.QueryRowContext(ctx, query, platform, handle).Scan(
		&p.ID, &p.Platform, &p.Handle, &p.URL, &ignored, &disabled,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsIgnored = ignored == 1
	p.Disabled = disabled == 1
	return &p, nil
}

// GetProgramByPlatformHandleAny fetches a program by platform and handle,
// including disabled/ignored programs. Returns nil if not found.
func (d *DB) GetProgramByPlatformHandleAny(ctx context.Context, platform, handle string) (*Program, error) {
	query := `SELECT id, platform, handle, url, is_ignored, disabled
		FROM programs
		WHERE LOWER(platform) = LOWER($1) AND LOWER(handle) = LOWER($2)
		LIMIT 1`

	var p Program
	var ignored, disabled int
	err := d.sql.QueryRowContext(ctx, query, platform, handle).Scan(
		&p.ID, &p.Platform, &p.Handle, &p.URL, &ignored, &disabled,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsIgnored = ignored == 1
	p.Disabled = disabled == 1
	return &p, nil
}

// ListProgramTargets returns all targets for a specific program.
// When rawMode is true, AI enhancements are ignored.
func (d *DB) ListProgramTargets(ctx context.Context, programID int64, rawMode bool) ([]ProgramTarget, error) {
	var query string
	if rawMode {
		query = `
			SELECT
				t.target AS target_display,
				t.target AS target_raw,
				t.category,
				COALESCE(t.description, '') AS description,
				t.in_scope,
				t.is_bbp
			FROM targets_raw t
			WHERE t.program_id = $1
			ORDER BY t.in_scope DESC, t.target
		`
	} else {
		query = `
			SELECT
				COALESCE(NULLIF(a.target_ai_normalized, ''), t.target) AS target_display,
				t.target AS target_raw,
				COALESCE(NULLIF(a.category, ''), t.category) AS category,
				COALESCE(t.description, '') AS description,
				COALESCE(a.in_scope, t.in_scope) AS in_scope,
				t.is_bbp
			FROM targets_raw t
			LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
			WHERE t.program_id = $1
			ORDER BY COALESCE(a.in_scope, t.in_scope) DESC,
				COALESCE(NULLIF(a.target_ai_normalized, ''), t.target)
		`
	}

	rows, err := d.sql.QueryContext(ctx, query, programID)
	if err != nil {
		return nil, fmt.Errorf("listing program targets: %w", err)
	}
	defer rows.Close()

	var targets []ProgramTarget
	for rows.Next() {
		var t ProgramTarget
		var inScope, isBBP int
		var descNS sql.NullString
		if err := rows.Scan(&t.TargetDisplay, &t.TargetRaw, &t.Category, &descNS, &inScope, &isBBP); err != nil {
			return nil, err
		}
		t.Description = descNS.String
		t.InScope = inScope == 1
		t.IsBBP = isBBP == 1
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// ListProgramTargetsFromHistory reconstructs a program's scope from scope_changes
// for removed programs whose targets have been hard-deleted from targets_raw.
// It picks the most recent "added" entry per (target_raw, category) pair,
// excluding category='program' rows.
func (d *DB) ListProgramTargetsFromHistory(ctx context.Context, platform, handle string) ([]ProgramTarget, error) {
	query := `
		SELECT
			COALESCE(NULLIF(c.target_ai_normalized, ''), NULLIF(c.target_normalized, ''), c.target_raw) AS target_display,
			c.target_raw,
			c.category,
			c.in_scope,
			c.is_bbp
		FROM scope_changes c
		INNER JOIN (
			SELECT target_raw, category, MAX(occurred_at) AS max_time
			FROM scope_changes
			WHERE LOWER(platform) = LOWER($1)
			AND LOWER(handle) = LOWER($2)
			AND category != 'program'
			AND change_type = 'added'
			GROUP BY target_raw, category
		) latest ON c.target_raw = latest.target_raw
			AND c.category = latest.category
			AND c.occurred_at = latest.max_time
		WHERE LOWER(c.platform) = LOWER($1)
		AND LOWER(c.handle) = LOWER($2)
		AND c.category != 'program'
		AND c.change_type = 'added'
		ORDER BY c.in_scope DESC, target_display
	`

	rows, err := d.sql.QueryContext(ctx, query, platform, handle)
	if err != nil {
		return nil, fmt.Errorf("listing program targets from history: %w", err)
	}
	defer rows.Close()

	var targets []ProgramTarget
	for rows.Next() {
		var t ProgramTarget
		var inScope, isBBP int
		if err := rows.Scan(&t.TargetDisplay, &t.TargetRaw, &t.Category, &inScope, &isBBP); err != nil {
			return nil, err
		}
		t.InScope = inScope == 1
		t.IsBBP = isBBP == 1
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// ListProgramChanges returns recent scope changes for a specific program.
func (d *DB) ListProgramChanges(ctx context.Context, platform, handle string, limit int) ([]Change, error) {
	var query string
	var args []interface{}
	if limit > 0 {
		query = `SELECT occurred_at, program_url, platform, handle,
			target_normalized, target_raw, target_ai_normalized,
			category, in_scope, is_bbp, change_type
			FROM scope_changes
			WHERE LOWER(platform) = LOWER($1) AND LOWER(handle) = LOWER($2)
			ORDER BY occurred_at DESC
			LIMIT $3`
		args = []interface{}{platform, handle, limit}
	} else {
		query = `SELECT occurred_at, program_url, platform, handle,
			target_normalized, target_raw, target_ai_normalized,
			category, in_scope, is_bbp, change_type
			FROM scope_changes
			WHERE LOWER(platform) = LOWER($1) AND LOWER(handle) = LOWER($2)
			ORDER BY occurred_at DESC`
		args = []interface{}{platform, handle}
	}

	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing program changes: %w", err)
	}
	defer rows.Close()

	var changes []Change
	for rows.Next() {
		var c Change
		var inScopeInt, isBBPInt int
		if err := rows.Scan(&c.OccurredAt, &c.ProgramURL, &c.Platform, &c.Handle,
			&c.TargetNormalized, &c.TargetRaw, &c.TargetAINormalized,
			&c.Category, &inScopeInt, &isBBPInt, &c.ChangeType); err != nil {
			return nil, fmt.Errorf("scanning program change: %w", err)
		}
		c.InScope = inScopeInt == 1
		c.IsBBP = isBBPInt == 1
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

// CountPrograms returns the number of active programs, optionally filtered by platform.
func (d *DB) CountPrograms(ctx context.Context, platform string) (int, error) {
	query := "SELECT COUNT(*) FROM programs WHERE disabled = 0 AND is_ignored = 0"
	args := []any{}
	if platform != "" {
		query += " AND LOWER(platform) = LOWER($1)"
		args = append(args, platform)
	}
	var count int
	err := d.sql.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// ListAllProgramSlugs returns platform+handle pairs for all active programs (used for sitemap).
func (d *DB) ListAllProgramSlugs(ctx context.Context) ([]ProgramSlug, error) {
	query := `SELECT platform, handle FROM programs WHERE is_ignored = 0 ORDER BY platform, handle`
	rows, err := d.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slugs []ProgramSlug
	for rows.Next() {
		var s ProgramSlug
		if err := rows.Scan(&s.Platform, &s.Handle); err != nil {
			return nil, err
		}
		slugs = append(slugs, s)
	}
	return slugs, rows.Err()
}

// GetAssetCountsByCategory returns a map of category->count for all in-scope assets.
// GetAssetCountsByCategory returns in-scope asset counts grouped by category.
// bbpFilter can be "bbp", "vdp", or "" for all.
func (d *DB) GetAssetCountsByCategory(ctx context.Context, bbpFilter string) (map[string]int, error) {
	programFilter := ""
	switch bbpFilter {
	case "bbp":
		programFilter = "AND EXISTS (SELECT 1 FROM targets_raw t2 WHERE t2.program_id = p.id AND t2.is_bbp = 1)"
	case "vdp":
		programFilter = "AND NOT EXISTS (SELECT 1 FROM targets_raw t2 WHERE t2.program_id = p.id AND t2.is_bbp = 1)"
	}

	query := fmt.Sprintf(`
		SELECT COALESCE(NULLIF(a.category, ''), t.category) AS cat,
			COUNT(*) AS cnt
		FROM targets_raw t
		JOIN programs p ON t.program_id = p.id
		LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
		WHERE p.disabled = 0 AND p.is_ignored = 0
		AND COALESCE(a.in_scope, t.in_scope) = 1
		%s
		GROUP BY cat
		ORDER BY cnt DESC
	`, programFilter)
	rows, err := d.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var cat string
		var cnt int
		if err := rows.Scan(&cat, &cnt); err != nil {
			return nil, err
		}
		if cat != "" {
			counts[cat] = cnt
		}
	}
	return counts, rows.Err()
}

// TargetCounts holds raw and AI-enhanced target row counts.
type TargetCounts struct {
	RawCount int
	AICount  int
}

// GetTargetCounts returns the total number of rows in targets_raw and targets_ai_enhanced.
func (d *DB) GetTargetCounts(ctx context.Context) (TargetCounts, error) {
	var tc TargetCounts
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM targets_raw`).Scan(&tc.RawCount)
	if err != nil {
		return tc, err
	}
	err = d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM targets_ai_enhanced`).Scan(&tc.AICount)
	return tc, err
}
