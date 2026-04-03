package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ianhomer/cicada/internal/model"
)

// CreateBuild inserts a new build record.
func (s *Store) CreateBuild(b *model.Build) error {
	_, err := s.db.Exec(`
		INSERT INTO builds (id, pipeline_name, status, ref, commit_sha, clone_url, created_at, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.PipelineName, string(b.Status), b.Ref, b.CommitSHA, b.CloneURL,
		b.CreatedAt, b.StartedAt, b.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	return nil
}

// GetBuild retrieves a build by ID.
func (s *Store) GetBuild(id string) (*model.Build, error) {
	row := s.db.QueryRow(`
		SELECT id, pipeline_name, status, ref, commit_sha, clone_url, created_at, started_at, finished_at
		FROM builds WHERE id = ?`, id)
	return scanBuild(row)
}

// ListBuilds returns all builds ordered by created_at descending.
func (s *Store) ListBuilds() ([]*model.Build, error) {
	rows, err := s.db.Query(`
		SELECT id, pipeline_name, status, ref, commit_sha, clone_url, created_at, started_at, finished_at
		FROM builds ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	defer rows.Close()

	var builds []*model.Build
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

// UpdateBuildStatus updates the status and optional timestamps of a build.
func (s *Store) UpdateBuildStatus(id string, status model.BuildStatus, startedAt, finishedAt *time.Time) error {
	res, err := s.db.Exec(`
		UPDATE builds SET status = ?, started_at = ?, finished_at = ? WHERE id = ?`,
		string(status), startedAt, finishedAt, id,
	)
	if err != nil {
		return fmt.Errorf("update build status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("build not found: %s", id)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBuild(s scanner) (*model.Build, error) {
	var b model.Build
	var status string
	var startedAt, finishedAt sql.NullTime
	err := s.Scan(
		&b.ID, &b.PipelineName, &status, &b.Ref, &b.CommitSHA, &b.CloneURL,
		&b.CreatedAt, &startedAt, &finishedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan build: %w", err)
	}
	b.Status = model.BuildStatus(status)
	if startedAt.Valid {
		b.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		b.FinishedAt = &finishedAt.Time
	}
	return &b, nil
}
