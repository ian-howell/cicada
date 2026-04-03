package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ianhomer/cicada/internal/model"
)

// CreateStepResult inserts a new step result record.
func (s *Store) CreateStepResult(sr *model.StepResult) error {
	_, err := s.db.Exec(`
		INSERT INTO step_results (build_id, step_name, status, exit_code, started_at, finished_at, log_file)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sr.BuildID, sr.StepName, string(sr.Status), sr.ExitCode,
		sr.StartedAt, sr.FinishedAt, sr.LogFile,
	)
	if err != nil {
		return fmt.Errorf("insert step result: %w", err)
	}
	return nil
}

// ListStepResults returns all step results for a build.
func (s *Store) ListStepResults(buildID string) ([]*model.StepResult, error) {
	rows, err := s.db.Query(`
		SELECT build_id, step_name, status, exit_code, started_at, finished_at, log_file
		FROM step_results WHERE build_id = ?`, buildID)
	if err != nil {
		return nil, fmt.Errorf("list step results: %w", err)
	}
	defer rows.Close()

	var results []*model.StepResult
	for rows.Next() {
		sr, err := scanStepResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, sr)
	}
	return results, rows.Err()
}

// UpdateStepResult updates a step result's status, exit code, and timestamps.
func (s *Store) UpdateStepResult(buildID, stepName string, status model.BuildStatus, exitCode int, startedAt, finishedAt *time.Time) error {
	_, err := s.db.Exec(`
		UPDATE step_results SET status = ?, exit_code = ?, started_at = ?, finished_at = ?
		WHERE build_id = ? AND step_name = ?`,
		string(status), exitCode, startedAt, finishedAt, buildID, stepName,
	)
	if err != nil {
		return fmt.Errorf("update step result: %w", err)
	}
	return nil
}

func scanStepResult(rows *sql.Rows) (*model.StepResult, error) {
	var sr model.StepResult
	var status string
	var startedAt, finishedAt sql.NullTime
	err := rows.Scan(
		&sr.BuildID, &sr.StepName, &status, &sr.ExitCode,
		&startedAt, &finishedAt, &sr.LogFile,
	)
	if err != nil {
		return nil, fmt.Errorf("scan step result: %w", err)
	}
	sr.Status = model.BuildStatus(status)
	if startedAt.Valid {
		sr.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		sr.FinishedAt = &finishedAt.Time
	}
	return &sr, nil
}
