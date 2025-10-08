package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS programs (
	id        INTEGER PRIMARY KEY,
	platform  TEXT NOT NULL,
	handle    TEXT NOT NULL,
	url       TEXT NOT NULL UNIQUE,
	first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	strict    INTEGER NOT NULL DEFAULT 0 CHECK (strict IN (0,1)),
	disabled  INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0,1)),
	is_ignored INTEGER NOT NULL DEFAULT 0 CHECK (is_ignored IN (0,1))
);
CREATE INDEX IF NOT EXISTS idx_programs_platform ON programs(platform);
CREATE INDEX IF NOT EXISTS idx_programs_url ON programs(url);
CREATE TABLE IF NOT EXISTS targets (
	id                INTEGER PRIMARY KEY,
	program_id        INTEGER NOT NULL,
	target_normalized TEXT NOT NULL,
	target_raw        TEXT NOT NULL,
	category          TEXT NOT NULL,
	description       TEXT,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	first_seen_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(program_id) REFERENCES programs(id),
	UNIQUE(program_id, target_raw, category)
);
CREATE INDEX IF NOT EXISTS idx_targets_program_id ON targets(program_id);
CREATE TABLE IF NOT EXISTS scope_changes (
	id                INTEGER PRIMARY KEY,
	occurred_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	program_url       TEXT NOT NULL,
	platform          TEXT NOT NULL,
	handle            TEXT NOT NULL,
	target_normalized TEXT NOT NULL,
	category          TEXT NOT NULL,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	change_type       TEXT NOT NULL CHECK (change_type IN ('added','updated','removed'))
);
CREATE INDEX IF NOT EXISTS idx_changes_time ON scope_changes(occurred_at);
CREATE INDEX IF NOT EXISTS idx_changes_program ON scope_changes(program_url, occurred_at);
`

func Open(path string) (*DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &DB{sql: db}, nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

func (d *DB) UpsertProgramEntries(ctx context.Context, programURL, platform, handle string, entries []Entry) ([]Change, error) {
	now := time.Now().UTC()

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Get or create program
	var programID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM programs WHERE url = ?", programURL).Scan(&programID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, "INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at) VALUES(?,?,?,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", platform, handle, programURL)
		if err != nil {
			return nil, fmt.Errorf("inserting program: %w", err)
		}
		programID, err = res.LastInsertId()
		if err != nil {
			return nil, err
		}
	} else {
		// Program exists, update last_seen_at
		_, err = tx.ExecContext(ctx, "UPDATE programs SET last_seen_at = CURRENT_TIMESTAMP, disabled = 0 WHERE id = ?", programID)
		if err != nil {
			return nil, err
		}
	}

	// 2. Get existing targets for this program
	rows, err := tx.QueryContext(ctx, "SELECT id, target_raw, target_normalized, category, in_scope, description, is_bbp FROM targets WHERE program_id = ?", programID)
	if err != nil {
		return nil, err
	}

	type existingTarget struct {
		ID                   int64
		Raw, Norm, Cat, Desc string
		InScope, IsBBP       bool
	}
	existingMap := make(map[string]existingTarget)

	for rows.Next() {
		var (
			id, inScope, isBBP int64
			raw, norm, cat     string
			desc               sql.NullString
		)
		if err = rows.Scan(&id, &raw, &norm, &cat, &inScope, &desc, &isBBP); err != nil {
			rows.Close()
			return nil, err
		}
		key := identityKey(raw, cat)
		existingMap[key] = existingTarget{ID: id, Raw: raw, Norm: norm, Cat: cat, Desc: desc.String, InScope: inScope == 1, IsBBP: isBBP == 1}
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	// 3. Insert or update new entries
	var changes []Change
	processedKeys := make(map[string]bool)

	for _, e := range entries {
		key := identityKey(e.TargetRaw, e.Category)
		if processedKeys[key] {
			continue // Skip duplicates within the same API response
		}

		inScopeInt := boolToInt(e.InScope)
		isBBPInt := boolToInt(e.IsBBP)
		ex, existed := existingMap[key]

		if !existed {
			_, err = tx.ExecContext(ctx, `INSERT INTO targets(program_id, target_normalized, target_raw, category, description, in_scope, is_bbp, first_seen_at, last_seen_at) VALUES(?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
				programID, e.TargetNormalized, e.TargetRaw, e.Category, nullIfEmpty(e.Description), inScopeInt, isBBPInt)
			if err != nil {
				return nil, err
			}
			changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetNormalized: e.TargetNormalized, Category: e.Category, InScope: e.InScope, IsBBP: e.IsBBP, ChangeType: "added"})
		} else {
			if ex.Desc != e.Description || ex.InScope != e.InScope || ex.IsBBP != e.IsBBP {
				_, err = tx.ExecContext(ctx, `UPDATE targets SET description = ?, in_scope = ?, is_bbp = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`, nullIfEmpty(e.Description), inScopeInt, isBBPInt, ex.ID)
				if err != nil {
					return nil, err
				}
				changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetNormalized: e.TargetNormalized, Category: e.Category, InScope: e.InScope, IsBBP: e.IsBBP, ChangeType: "updated"})
			} else {
				// Just update last_seen_at
				_, err = tx.ExecContext(ctx, `UPDATE targets SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`, ex.ID)
				if err != nil {
					return nil, err
				}
			}
		}
		processedKeys[key] = true
	}

	// 4. Sweep for removed targets
	for key, ex := range existingMap {
		if !processedKeys[key] {
			// This target existed in the DB but wasn't in the latest poll, so it's removed.
			_, err = tx.ExecContext(ctx, `DELETE FROM targets WHERE id = ?`, ex.ID)
			if err != nil {
				return nil, err
			}
			_, ierr := tx.ExecContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, category, in_scope, is_bbp, change_type) VALUES(CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, ?, 'removed')`, programURL, platform, handle, ex.Norm, ex.Cat, boolToInt(ex.InScope), boolToInt(ex.IsBBP))
			if ierr != nil {
				return nil, ierr
			}
			changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetNormalized: ex.Norm, Category: ex.Cat, InScope: ex.InScope, IsBBP: ex.IsBBP, ChangeType: "removed"})
		}
	}

	return changes, tx.Commit()
}

// SetProgramIgnoredStatus sets the is_ignored flag for a program.
func (d *DB) SetProgramIgnoredStatus(ctx context.Context, programURL string, ignored bool) error {
	res, err := d.sql.ExecContext(ctx, "UPDATE programs SET is_ignored = ? WHERE url LIKE ?", boolToInt(ignored), fmt.Sprintf("%%%s%%", programURL))
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no program found matching URL pattern: %s", programURL)
	}
	return nil
}

// GetIgnoredPrograms returns a map of program URLs that are marked as ignored for a specific platform.
func (d *DB) GetIgnoredPrograms(ctx context.Context, platform string) (map[string]bool, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT url FROM programs WHERE platform = ? AND is_ignored = 1", platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ignoredMap := make(map[string]bool)
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		ignoredMap[url] = true
	}
	return ignoredMap, rows.Err()
}

// SyncPlatformPrograms marks programs that are no longer returned by a platform's API as 'disabled'
// and logs their removal as a single event, preventing spam from individual target removals.
func (d *DB) SyncPlatformPrograms(ctx context.Context, platform string, polledProgramURLs []string) ([]Change, error) {
	now := time.Now().UTC()
	var changes []Change

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Create a temporary table to hold the URLs from the latest poll
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE polled_urls (url TEXT NOT NULL PRIMARY KEY)`)
	if err != nil {
		return nil, fmt.Errorf("creating temp table: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO polled_urls (url) VALUES (?)`)
	if err != nil {
		return nil, fmt.Errorf("preparing insert statement: %w", err)
	}
	defer stmt.Close()

	for _, url := range polledProgramURLs {
		if _, err := stmt.ExecContext(ctx, url); err != nil {
			return nil, fmt.Errorf("inserting into temp table: %w", err)
		}
	}

	// Find programs in the DB for this platform that are NOT in the polled list
	rows, err := tx.QueryContext(ctx, `
		SELECT p.id, p.url, p.handle
		FROM programs p
		LEFT JOIN polled_urls pu ON p.url = pu.url
		WHERE p.platform = ? AND p.disabled = 0 AND p.is_ignored = 0 AND pu.url IS NULL
	`, platform)
	if err != nil {
		return nil, fmt.Errorf("querying for removed programs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var programID int64
		var programURL, handle string
		if err := rows.Scan(&programID, &programURL, &handle); err != nil {
			return nil, err
		}

		// Mark the program as disabled
		if _, err := tx.ExecContext(ctx, `UPDATE programs SET disabled = 1 WHERE id = ?`, programID); err != nil {
			return nil, fmt.Errorf("disabling program %d: %w", programID, err)
		}

		// Delete associated targets
		if _, err := tx.ExecContext(ctx, `DELETE FROM targets WHERE program_id = ?`, programID); err != nil {
			return nil, fmt.Errorf("deleting targets for program %d: %w", programID, err)
		}

		// Create a single "removed" change event for the entire program
		change := Change{
			OccurredAt:       now,
			ProgramURL:       programURL,
			Platform:         platform,
			Handle:           handle,
			TargetNormalized: programURL, // Use the program URL as the "target"
			Category:         "program",
			InScope:          false,
			IsBBP:            false,
			ChangeType:       "removed",
		}
		changes = append(changes, change)

		_, ierr := tx.ExecContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, category, in_scope, is_bbp, change_type) VALUES(CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, ?, 'removed')`,
			programURL, platform, handle, programURL, "program", boolToInt(false), boolToInt(false))
		if ierr != nil {
			return nil, ierr
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return changes, tx.Commit()
}

// BuildEntries unchanged...
func BuildEntries(programURL, platform, handle string, items []TargetItem) ([]Entry, error) {
	if programURL == "" || platform == "" {
		return nil, errors.New("invalid program identifiers")
	}
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		normalized := NormalizeTarget(it.URI)
		normalizedCategory := scope.NormalizeCategory(it.Category)

		out = append(out, Entry{
			ProgramURL:       NormalizeProgramURL(programURL),
			Platform:         platform,
			Handle:           handle,
			TargetNormalized: normalized,
			TargetRaw:        it.URI,
			Category:         normalizedCategory,
			Description:      it.Description,
			InScope:          it.InScope,
			IsBBP:            it.IsBBP,
		})
	}
	return out, nil
}

// ListOptions controls selection when listing entries.
type ListOptions struct {
	Platform       string
	ProgramFilter  string
	Since          time.Time
	IncludeOOS     bool
	IncludeIgnored bool
}

// ListEntries returns current entries matching filters.
func (d *DB) ListEntries(ctx context.Context, opts ListOptions) ([]Entry, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if opts.Platform != "" && opts.Platform != "all" {
		where += " AND p.platform = ?"
		args = append(args, opts.Platform)
	}
	if opts.ProgramFilter != "" {
		where += " AND p.url LIKE ?"
		args = append(args, fmt.Sprintf("%%%s%%", opts.ProgramFilter))
	}
	if !opts.IncludeOOS {
		where += " AND t.in_scope = 1"
	}
	if !opts.IncludeIgnored {
		where += " AND p.is_ignored = 0"
	}
	if !opts.Since.IsZero() {
		where += " AND t.last_seen_at >= ?"
		args = append(args, opts.Since.UTC())
	}

	q := `
		SELECT p.url, p.platform, p.handle, t.target_normalized, t.target_raw, t.category, t.description, t.in_scope, t.is_bbp 
		FROM targets t JOIN programs p ON t.program_id = p.id 
	` + where + " ORDER BY p.url, t.target_normalized"

	rows, err := d.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var inScopeInt, isBBPInt int
		var rawNS, descNS sql.NullString
		if err := rows.Scan(&e.ProgramURL, &e.Platform, &e.Handle, &e.TargetNormalized, &rawNS, &e.Category, &descNS, &inScopeInt, &isBBPInt); err != nil {
			return nil, err
		}
		e.TargetRaw = rawNS.String
		e.Description = descNS.String
		e.InScope = inScopeInt == 1
		e.IsBBP = isBBPInt == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListRecentChanges returns the most recent N changes across all programs.
func (d *DB) ListRecentChanges(ctx context.Context, limit int) ([]Change, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT occurred_at, program_url, platform, handle, target_normalized, category, in_scope, is_bbp, change_type FROM scope_changes ORDER BY occurred_at DESC LIMIT ?"
	rows, err := d.sql.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := []Change{}
	for rows.Next() {
		var c Change
		var occurredAtStr string
		var inScopeInt, isBBPInt int
		if err := rows.Scan(&occurredAtStr, &c.ProgramURL, &c.Platform, &c.Handle, &c.TargetNormalized, &c.Category, &inScopeInt, &isBBPInt, &c.ChangeType); err != nil {
			return nil, err
		}
		if t, perr := time.Parse("2006-01-02 15:04:05", occurredAtStr); perr == nil {
			c.OccurredAt = t
		} else if t2, perr2 := time.Parse(time.RFC3339, occurredAtStr); perr2 == nil {
			c.OccurredAt = t2
		}
		c.InScope = inScopeInt == 1
		c.IsBBP = isBBPInt == 1
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

type PlatformStats struct {
	Platform        string
	ProgramCount    int
	InScopeCount    int
	OutOfScopeCount int
}

func (d *DB) GetStats(ctx context.Context) ([]PlatformStats, error) {
	query := `
		SELECT
			p.platform,
			COUNT(DISTINCT p.id),
			SUM(CASE WHEN t.in_scope = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN t.in_scope = 0 THEN 1 ELSE 0 END)
		FROM
			programs p JOIN targets t ON p.id = t.program_id
		WHERE
			p.is_ignored = 0
		GROUP BY
			p.platform
		ORDER BY
			p.platform;
	`
	rows, err := d.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PlatformStats
	for rows.Next() {
		var s PlatformStats
		if err := rows.Scan(&s.Platform, &s.ProgramCount, &s.InScopeCount, &s.OutOfScopeCount); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

func (d *DB) SearchTargets(ctx context.Context, searchTerm string) ([]Entry, error) {
	likeQuery := fmt.Sprintf("%%%s%%", searchTerm)

	query := `
		SELECT p.url, p.platform, p.handle, t.target_normalized, t.target_raw, t.category, t.description, t.in_scope, t.is_bbp, 0 as is_historical
		FROM targets t 
		JOIN programs p ON t.program_id = p.id 
		WHERE (t.target_normalized LIKE ? OR t.description LIKE ?) AND p.is_ignored = 0

		UNION

		SELECT c.program_url, c.platform, c.handle, c.target_normalized, '' as target_raw, c.category, '' as description, c.in_scope, c.is_bbp, 1 as is_historical
		FROM scope_changes c
		WHERE c.target_normalized LIKE ?;
	`

	rows, err := d.sql.QueryContext(ctx, query, likeQuery, likeQuery, likeQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	seen := make(map[string]bool)

	for rows.Next() {
		var e Entry
		var inScopeInt, isHistoricalInt, isBBPInt int
		var rawNS, descNS sql.NullString
		if err := rows.Scan(&e.ProgramURL, &e.Platform, &e.Handle, &e.TargetNormalized, &rawNS, &e.Category, &descNS, &inScopeInt, &isBBPInt, &isHistoricalInt); err != nil {
			return nil, err
		}
		e.TargetRaw = rawNS.String
		e.Description = descNS.String
		e.InScope = inScopeInt == 1
		e.IsBBP = isBBPInt == 1
		e.IsHistorical = isHistoricalInt == 1

		// The UNION can return duplicates, so we'll filter them here.
		key := fmt.Sprintf("%s|%s|%s", e.ProgramURL, e.TargetNormalized, e.Category)
		if !seen[key] {
			out = append(out, e)
			seen[key] = true
		}
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func identityKey(raw, category string) string {
	if raw == "" || category == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s", raw, category)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
