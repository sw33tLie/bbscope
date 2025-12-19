package storage

import (
	"context"
	"fmt"
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
