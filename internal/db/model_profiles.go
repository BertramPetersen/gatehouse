package db

import (
	"database/sql"
	"errors"
	"fmt"
)

func (d *DB) GetRepoModelChoices(repoID string) (map[string]string, error) {
	rows, err := d.sql.Query(`SELECT step, profile FROM repo_model_choices WHERE repo_id = ?`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	choices := map[string]string{}
	for rows.Next() {
		var step, profile string
		if err := rows.Scan(&step, &profile); err != nil {
			return nil, err
		}
		choices[step] = profile
	}
	return choices, rows.Err()
}

// DeleteRepoModelChoices forgets future-run selections, never existing pins.
func (d *DB) DeleteRepoModelChoices(repoID string, steps []string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, step := range steps {
		if _, err := tx.Exec(`DELETE FROM repo_model_choices WHERE repo_id = ? AND step = ?`, repoID, step); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) InsertRunModelPlan(runID, plan string) error {
	_, err := d.sql.Exec(`INSERT INTO run_model_plans (run_id, plan) VALUES (?, ?)`, runID, plan)
	return err
}

func (d *DB) GetRunModelPlan(runID string) (string, error) {
	var plan string
	err := d.sql.QueryRow(`SELECT plan FROM run_model_plans WHERE run_id = ?`, runID).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return plan, err
}

// SaveModelChoices uses the old pin as a compare-and-swap token. A second
// client cannot overwrite a first client's choices or retarget a running run.
func (d *DB) SaveModelChoices(runID, repoID, oldPlan, newPlan string, choices map[string]string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE run_model_plans SET plan = ? WHERE run_id = ? AND plan = ?
 AND EXISTS (SELECT 1 FROM runs WHERE id = ? AND repo_id = ? AND status = 'pending')
 AND NOT EXISTS (SELECT 1 FROM step_results WHERE run_id = ?)`, newPlan, runID, oldPlan, runID, repoID, runID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("model setup changed; refresh the run before saving")
	}
	for step, profile := range choices {
		if _, err := tx.Exec(`INSERT INTO repo_model_choices (repo_id,step,profile) VALUES (?,?,?)
 ON CONFLICT(repo_id,step) DO UPDATE SET profile = excluded.profile`, repoID, step, profile); err != nil {
			return err
		}
	}
	return tx.Commit()
}
