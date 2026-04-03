CREATE TABLE IF NOT EXISTS builds (
    id           TEXT PRIMARY KEY,
    pipeline_name TEXT NOT NULL,
    status       TEXT NOT NULL,
    ref          TEXT NOT NULL,
    commit_sha   TEXT NOT NULL,
    clone_url    TEXT NOT NULL,
    created_at   DATETIME NOT NULL,
    started_at   DATETIME,
    finished_at  DATETIME
);

CREATE TABLE IF NOT EXISTS step_results (
    build_id    TEXT NOT NULL,
    step_name   TEXT NOT NULL,
    status      TEXT NOT NULL,
    exit_code   INTEGER NOT NULL DEFAULT 0,
    started_at  DATETIME,
    finished_at DATETIME,
    log_file    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (build_id, step_name),
    FOREIGN KEY (build_id) REFERENCES builds(id)
);
