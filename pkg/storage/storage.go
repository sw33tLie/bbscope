package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	_ "modernc.org/sqlite"
)

var (
	// ErrAbortingScopeWipe is returned when an update would wipe all targets from a program.
	ErrAbortingScopeWipe = errors.New("aborting update to prevent wiping out all targets for a program")
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
	UNIQUE(program_id, target_raw)
);
CREATE INDEX IF NOT EXISTS idx_targets_program_id ON targets(program_id);
CREATE TABLE IF NOT EXISTS scope_changes (
	id                INTEGER PRIMARY KEY,
	occurred_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	program_url       TEXT NOT NULL,
	platform          TEXT NOT NULL,
	handle            TEXT NOT NULL,
	target_normalized TEXT NOT NULL,
	target_raw        TEXT NOT NULL DEFAULT '',
	category          TEXT NOT NULL,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	change_type       TEXT NOT NULL CHECK (change_type IN ('added','updated','removed'))
);
CREATE INDEX IF NOT EXISTS idx_changes_time ON scope_changes(occurred_at);
CREATE INDEX IF NOT EXISTS idx_changes_program ON scope_changes(program_url, occurred_at);
`

func Open(path string, timeout int) (*DB, error) {
	if timeout <= 0 {
		timeout = 5000 // Default to 5 seconds if not specified or invalid
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)", path, timeout)
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

// getOrCreateProgram handles the atomic retrieval or creation of a program entry.
// It uses a short-lived transaction to prevent race conditions and minimize lock time.
func (d *DB) getOrCreateProgram(ctx context.Context, programURL, platform, handle string) (int64, error) {
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var programID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM programs WHERE url = ?", programURL).Scan(&programID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err // A real error occurred.
	}

	if errors.Is(err, sql.ErrNoRows) {
		// Program doesn't exist, create it.
		res, err := tx.ExecContext(ctx, "INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at) VALUES(?,?,?,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", platform, handle, programURL)
		if err != nil {
			return 0, fmt.Errorf("inserting program: %w", err)
		}
		programID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		// Program exists, update last_seen_at.
		_, err = tx.ExecContext(ctx, "UPDATE programs SET last_seen_at = CURRENT_TIMESTAMP, disabled = 0 WHERE id = ?", programID)
		if err != nil {
			return 0, err
		}
	}

	return programID, tx.Commit()
}

func (d *DB) UpsertProgramEntries(ctx context.Context, programURL, platform, handle string, entries []Entry) ([]Change, error) {
	now := time.Now().UTC()

	// 1. Get or create program in its own short transaction to minimize lock time.
	programID, err := d.getOrCreateProgram(ctx, programURL, platform, handle)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create program: %w", err)
	}

	// 2. Get existing targets for this program (this is a read, no transaction needed in WAL mode)
	rows, err := d.sql.QueryContext(ctx, "SELECT id, target_raw, target_normalized, category, in_scope, description, is_bbp FROM targets WHERE program_id = ?", programID)
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
		key := identityKey(raw)
		existingMap[key] = existingTarget{ID: id, Raw: raw, Norm: norm, Cat: cat, Desc: desc.String, InScope: inScope == 1, IsBBP: isBBP == 1}
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	// SAFETY CHECK: If the incoming data has zero entries, but we know in the DB
	// that this program HAS entries, we abort. This prevents a broken poller from wiping a scope.
	if len(entries) == 0 && len(existingMap) > 0 {
		return nil, ErrAbortingScopeWipe
	}

	// 3. Perform comparison and prepare changes in memory (no DB lock)
	var changes []Change
	processedKeys := make(map[string]bool)

	toAdd := []Entry{}
	toUpdate := []struct {
		entry Entry
		id    int64
	}{}
	toTouch := []int64{}

	for _, e := range entries {
		key := identityKey(e.TargetRaw)
		if processedKeys[key] {
			continue // Skip duplicates within the same API response
		}
		processedKeys[key] = true

		ex, existed := existingMap[key]
		if !existed {
			toAdd = append(toAdd, e)
			changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetRaw: e.TargetRaw, TargetNormalized: e.TargetNormalized, Category: e.Category, InScope: e.InScope, IsBBP: e.IsBBP, ChangeType: "added"})
		} else {
			if ex.Desc != e.Description || ex.InScope != e.InScope || ex.IsBBP != e.IsBBP {
				toUpdate = append(toUpdate, struct {
					entry Entry
					id    int64
				}{entry: e, id: ex.ID})
				changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetRaw: e.TargetRaw, TargetNormalized: e.TargetNormalized, Category: e.Category, InScope: e.InScope, IsBBP: e.IsBBP, ChangeType: "updated"})
			} else {
				toTouch = append(toTouch, ex.ID)
			}
		}
	}

	toRemove := []existingTarget{}
	for key, ex := range existingMap {
		if !processedKeys[key] {
			toRemove = append(toRemove, ex)
			changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetRaw: ex.Raw, TargetNormalized: ex.Norm, Category: ex.Cat, InScope: ex.InScope, IsBBP: ex.IsBBP, ChangeType: "removed"})
		}
	}

	// 4. Start a transaction for all the batched write operations
	if len(toAdd) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO targets(program_id, target_normalized, target_raw, category, description, in_scope, is_bbp, first_seen_at, last_seen_at) VALUES(?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, e := range toAdd {
			_, err := stmt.ExecContext(ctx, programID, e.TargetNormalized, e.TargetRaw, e.Category, nullIfEmpty(e.Description), boolToInt(e.InScope), boolToInt(e.IsBBP))
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	// Batch Updates
	if len(toUpdate) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets SET description = ?, in_scope = ?, is_bbp = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, u := range toUpdate {
			_, err := stmt.ExecContext(ctx, nullIfEmpty(u.entry.Description), boolToInt(u.entry.InScope), boolToInt(u.entry.IsBBP), u.id)
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	// Batch Touches (update last_seen_at)
	if len(toTouch) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, id := range toTouch {
			_, err := stmt.ExecContext(ctx, id)
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	// Batch Deletes and log changes
	if len(toRemove) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		delStmt, err := tx.PrepareContext(ctx, `DELETE FROM targets WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}

		for _, ex := range toRemove {
			if _, err := delStmt.ExecContext(ctx, ex.ID); err != nil {
				delStmt.Close()
				tx.Rollback()
				return nil, err
			}
		}
		delStmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	return changes, nil
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

// GetActiveProgramCount returns the number of active (not disabled, not ignored) programs for a platform.
func (d *DB) GetActiveProgramCount(ctx context.Context, platform string) (int, error) {
	var count int
	err := d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM programs WHERE platform = ? AND disabled = 0 AND is_ignored = 0", platform).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// SyncPlatformPrograms marks programs that are no longer returned by a platform's API as 'disabled'
// and logs their removal as a single event, preventing spam from individual target removals.
func (d *DB) SyncPlatformPrograms(ctx context.Context, platform string, polledProgramURLs []string) ([]Change, error) {
	now := time.Now().UTC()
	var changes []Change

	// 1. Create a set of polled URLs for efficient lookup.
	polledURLSet := make(map[string]struct{}, len(polledProgramURLs))
	for _, url := range polledProgramURLs {
		polledURLSet[url] = struct{}{}
	}

	// 2. Get all active programs for this platform from the DB (read operation, no transaction needed).
	rows, err := d.sql.QueryContext(ctx, `
		SELECT p.id, p.url, p.handle
		FROM programs p
		WHERE p.platform = ? AND p.disabled = 0 AND p.is_ignored = 0
	`, platform)
	if err != nil {
		return nil, fmt.Errorf("querying for active programs: %w", err)
	}
	defer rows.Close()

	type programToRemove struct {
		ID     int64
		URL    string
		Handle string
	}
	var toRemove []programToRemove

	for rows.Next() {
		var p programToRemove
		if err := rows.Scan(&p.ID, &p.URL, &p.Handle); err != nil {
			return nil, err
		}
		if _, found := polledURLSet[p.URL]; !found {
			toRemove = append(toRemove, p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 3. For each program that was not in the latest poll, process its removal
	// in its own short-lived transaction to avoid long-held locks.
	for _, p := range toRemove {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, fmt.Errorf("starting transaction for program removal %d: %w", p.ID, err)
		}

		// Mark the program as disabled
		if _, err := tx.ExecContext(ctx, `UPDATE programs SET disabled = 1 WHERE id = ?`, p.ID); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("disabling program %d: %w", p.ID, err)
		}

		// Delete associated targets
		if _, err := tx.ExecContext(ctx, `DELETE FROM targets WHERE program_id = ?`, p.ID); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("deleting targets for program %d: %w", p.ID, err)
		}

		// Create a single "removed" change event for the entire program
		change := Change{
			OccurredAt:       now,
			ProgramURL:       p.URL,
			Platform:         platform,
			Handle:           p.Handle,
			TargetNormalized: p.URL, // Use the program URL as the "target"
			TargetRaw:        p.URL,
			Category:         "program",
			InScope:          false,
			IsBBP:            false,
			ChangeType:       "removed",
		}
		changes = append(changes, change)

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction for program removal %d: %w", p.ID, err)
		}
	}

	return changes, nil
}

func (d *DB) LogChanges(ctx context.Context, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, target_raw, category, in_scope, is_bbp, change_type) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range changes {
		_, err := stmt.ExecContext(ctx, c.OccurredAt, c.ProgramURL, c.Platform, c.Handle, c.TargetNormalized, c.TargetRaw, c.Category, boolToInt(c.InScope), boolToInt(c.IsBBP), c.ChangeType)
		if err != nil {
			return err // Rollback will be called
		}
	}

	return tx.Commit()
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

// AddCustomTarget adds a single target for a custom program.
func (d *DB) AddCustomTarget(ctx context.Context, target, category string) error {
	platform := "custom"
	// Sanitize target to avoid weird characters in URL
	safeTarget := url.PathEscape(target)
	programURL := "custom://" + safeTarget
	handle := target

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var programID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM programs WHERE url = ?", programURL).Scan(&programID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, "INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at) VALUES(?,?,?,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", platform, handle, programURL)
		if err != nil {
			return fmt.Errorf("inserting custom program: %w", err)
		}
		programID, err = res.LastInsertId()
		if err != nil {
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE programs SET last_seen_at = CURRENT_TIMESTAMP, disabled = 0 WHERE id = ?", programID)
		if err != nil {
			return err
		}
	}

	normalizedTarget := NormalizeTarget(target)

	var targetID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM targets WHERE program_id = ? AND target_raw = ?", programID, target).Scan(&targetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO targets(program_id, target_normalized, target_raw, category, in_scope, is_bbp, first_seen_at, last_seen_at)
			VALUES(?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			programID, normalizedTarget, target, category, boolToInt(true), boolToInt(false))
		if err != nil {
			return fmt.Errorf("inserting custom target: %w", err)
		}
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE targets SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?", targetID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListRecentChanges returns the most recent N changes across all programs.
func (d *DB) ListRecentChanges(ctx context.Context, limit int) ([]Change, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT occurred_at, program_url, platform, handle, target_normalized, target_raw, category, in_scope, is_bbp, change_type FROM scope_changes ORDER BY occurred_at DESC LIMIT ?"
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
		if err := rows.Scan(&occurredAtStr, &c.ProgramURL, &c.Platform, &c.Handle, &c.TargetNormalized, &c.TargetRaw, &c.Category, &inScopeInt, &isBBPInt, &c.ChangeType); err != nil {
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
		WHERE (t.target_normalized LIKE ? OR t.description LIKE ? OR p.url LIKE ?) AND p.is_ignored = 0

		UNION

		SELECT c.program_url, c.platform, c.handle, c.target_normalized, '' as target_raw, c.category, '' as description, c.in_scope, c.is_bbp, 1 as is_historical
		FROM scope_changes c
		WHERE c.target_normalized LIKE ? OR c.program_url LIKE ?;
	`

	rows, err := d.sql.QueryContext(ctx, query, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery)
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

func identityKey(raw string) string {
	if raw == "" {
		return ""
	}
	return raw
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
