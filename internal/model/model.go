package model

import "time"

// BuildStatus represents the lifecycle state of a build or step.
type BuildStatus string

const (
	StatusPending   BuildStatus = "pending"
	StatusRunning   BuildStatus = "running"
	StatusSuccess   BuildStatus = "success"
	StatusFailure   BuildStatus = "failure"
	StatusCancelled BuildStatus = "cancelled"
)

// EventType represents the kind of forge event that triggered a build.
type EventType string

const (
	EventPush        EventType = "push"
	EventPullRequest EventType = "pull_request"
	EventTag         EventType = "tag"
)

// ForgeEvent is the normalized representation of a webhook event from any forge.
type ForgeEvent struct {
	Type      EventType
	Repo      string
	CloneURL  string
	Ref       string
	CommitSHA string
	Sender    string
}

// Pipeline is parsed from a .cicada/*.yml file at build time. Not stored in DB.
type Pipeline struct {
	Name     string
	Image    string // default image for all steps
	Triggers []EventType
	Steps    []Step
}

// Step is one unit of work in a pipeline.
type Step struct {
	Name      string
	Image     string // resolved at parse time; always non-empty
	Commands  []string
	Env       map[string]string
	DependsOn []string
}

// Build is a single execution of a pipeline, stored in the database.
type Build struct {
	ID           string
	PipelineName string
	Status       BuildStatus
	Ref          string
	CommitSHA    string
	CloneURL     string
	CreatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// StepResult is the result of executing one step in a build, stored in the database.
type StepResult struct {
	BuildID    string
	StepName   string
	Status     BuildStatus
	ExitCode   int
	StartedAt  *time.Time
	FinishedAt *time.Time
	LogFile    string // path relative to data dir, e.g. "logs/<build-id>/<step-name>.log"
}
