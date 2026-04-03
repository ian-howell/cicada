package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ian-howell/cicada/internal/model"
)

// GitHubProvider handles GitHub webhook events.
type GitHubProvider struct {
	secret string
}

// NewGitHubProvider creates a GitHub provider with the given webhook secret.
func NewGitHubProvider(secret string) *GitHubProvider {
	if secret == "" {
		panic("webhook secret must not be empty")
	}
	return &GitHubProvider{secret: secret}
}

// Name returns the provider's identifier.
func (p *GitHubProvider) Name() string { return "github" }

// ParseWebhook validates the HMAC signature and parses the event payload.
func (p *GitHubProvider) ParseWebhook(r *http.Request) (*model.ForgeEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}

	if err := p.validateSignature(r.Header.Get("X-Hub-Signature-256"), body); err != nil {
		return nil, err
	}

	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "push":
		return p.parsePush(body)
	case "pull_request":
		return p.parsePullRequest(body)
	default:
		return nil, fmt.Errorf("unsupported event type: %q", eventType)
	}
}

func (p *GitHubProvider) validateSignature(signature string, body []byte) error {
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("missing or malformed X-Hub-Signature-256 header")
	}
	expected := p.computeHMAC(body)
	actual := strings.TrimPrefix(signature, "sha256=")
	actualBytes, err := hex.DecodeString(actual)
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	if !hmac.Equal(expected, actualBytes) {
		return fmt.Errorf("webhook signature mismatch")
	}
	return nil
}

func (p *GitHubProvider) computeHMAC(body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(p.secret))
	mac.Write(body)
	return mac.Sum(nil)
}

type githubPushPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func (p *GitHubProvider) parsePush(body []byte) (*model.ForgeEvent, error) {
	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse push payload: %w", err)
	}

	eventType := model.EventPush
	if strings.HasPrefix(payload.Ref, "refs/tags/") {
		eventType = model.EventTag
	}

	return &model.ForgeEvent{
		Type:      eventType,
		Repo:      payload.Repository.FullName,
		CloneURL:  payload.Repository.CloneURL,
		Ref:       payload.Ref,
		CommitSHA: payload.After,
		Sender:    payload.Sender.Login,
	}, nil
}

type githubPRPayload struct {
	PullRequest struct {
		Head struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Repo struct {
				CloneURL string `json:"clone_url"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
	} `json:"pull_request"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

// parsePullRequest handles pull_request events. All actions (opened, closed,
// synchronize, etc.) are normalized to EventPullRequest; action filtering is
// a future enhancement.
func (p *GitHubProvider) parsePullRequest(body []byte) (*model.ForgeEvent, error) {
	var payload githubPRPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse pull_request payload: %w", err)
	}
	return &model.ForgeEvent{
		Type:      model.EventPullRequest,
		Repo:      payload.PullRequest.Base.Repo.FullName,
		CloneURL:  payload.PullRequest.Base.Repo.CloneURL,
		Ref:       payload.PullRequest.Head.Ref,
		CommitSHA: payload.PullRequest.Head.SHA,
		Sender:    payload.Sender.Login,
	}, nil
}
