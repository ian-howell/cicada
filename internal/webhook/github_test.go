package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ian-howell/cicada/internal/model"
)

const testSecret = "testsecret"

func signPayload(t *testing.T, secret, payload string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestGitHubProvider_ParseWebhook_Push(t *testing.T) {
	payload := `{
		"ref": "refs/heads/main",
		"after": "abc123def456",
		"repository": {
			"full_name": "example/repo",
			"clone_url": "https://github.com/example/repo.git"
		},
		"sender": {"login": "octocat"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signPayload(t, testSecret, payload))

	p := NewGitHubProvider(testSecret)
	event, err := p.ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}

	if event.Type != model.EventPush {
		t.Errorf("Type = %q, want %q", event.Type, model.EventPush)
	}
	if event.Ref != "refs/heads/main" {
		t.Errorf("Ref = %q, want %q", event.Ref, "refs/heads/main")
	}
	if event.CommitSHA != "abc123def456" {
		t.Errorf("CommitSHA = %q, want %q", event.CommitSHA, "abc123def456")
	}
	if event.CloneURL != "https://github.com/example/repo.git" {
		t.Errorf("CloneURL = %q, want %q", event.CloneURL, "https://github.com/example/repo.git")
	}
}

func TestGitHubProvider_ParseWebhook_Tag(t *testing.T) {
	payload := `{
		"ref": "refs/tags/v1.0.0",
		"after": "deadbeef",
		"repository": {
			"full_name": "example/repo",
			"clone_url": "https://github.com/example/repo.git"
		},
		"sender": {"login": "octocat"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signPayload(t, testSecret, payload))

	p := NewGitHubProvider(testSecret)
	event, err := p.ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if event.Type != model.EventTag {
		t.Errorf("Type = %q, want %q", event.Type, model.EventTag)
	}
}

func TestGitHubProvider_ParseWebhook_BadSignature(t *testing.T) {
	payload := `{"ref":"refs/heads/main","after":"abc","repository":{"full_name":"a/b","clone_url":"https://github.com/a/b.git"},"sender":{"login":"u"}}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256=badhash")

	p := NewGitHubProvider(testSecret)
	_, err := p.ParseWebhook(req)
	if err == nil {
		t.Error("ParseWebhook() expected error for bad signature, got nil")
	}
}

func TestGitHubProvider_ParseWebhook_MissingSignatureHeader(t *testing.T) {
	payload := `{"ref":"refs/heads/main","after":"abc","repository":{"full_name":"a/b","clone_url":"https://github.com/a/b.git"},"sender":{"login":"u"}}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "push")
	// No X-Hub-Signature-256 header set

	p := NewGitHubProvider(testSecret)
	_, err := p.ParseWebhook(req)
	if err == nil {
		t.Error("ParseWebhook() expected error for missing signature header, got nil")
	}
}

func TestGitHubProvider_ParseWebhook_WrongHMAC(t *testing.T) {
	payload := `{"ref":"refs/heads/main","after":"abc","repository":{"full_name":"a/b","clone_url":"https://github.com/a/b.git"},"sender":{"login":"u"}}`

	// Sign with a different secret to produce valid hex but wrong HMAC value
	wrongSig := signPayload(t, "wrongsecret", payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", wrongSig)

	p := NewGitHubProvider(testSecret)
	_, err := p.ParseWebhook(req)
	if err == nil {
		t.Error("ParseWebhook() expected error for HMAC mismatch, got nil")
	}
}

func TestGitHubProvider_ParseWebhook_PullRequest(t *testing.T) {
	payload := `{
		"action": "opened",
		"pull_request": {
			"head": {"sha": "pr123sha", "ref": "feature-branch"},
			"base": {"repo": {"clone_url": "https://github.com/example/repo.git", "full_name": "example/repo"}}
		},
		"sender": {"login": "octocat"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", signPayload(t, testSecret, payload))

	p := NewGitHubProvider(testSecret)
	event, err := p.ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if event.Type != model.EventPullRequest {
		t.Errorf("Type = %q, want %q", event.Type, model.EventPullRequest)
	}
	if event.CommitSHA != "pr123sha" {
		t.Errorf("CommitSHA = %q, want %q", event.CommitSHA, "pr123sha")
	}
}

func TestRegistryFromEnv(t *testing.T) {
	t.Setenv("CICADA_GITHUB_WEBHOOK_SECRET", "mysecret")
	r := NewRegistryFromEnv()
	p, ok := r.Get("github")
	if !ok {
		t.Fatal("expected github provider in registry")
	}
	if p.Name() != "github" {
		t.Errorf("Name() = %q, want %q", p.Name(), "github")
	}
}

func TestRegistryFromEnv_NoSecret(t *testing.T) {
	os.Unsetenv("CICADA_GITHUB_WEBHOOK_SECRET")
	r := NewRegistryFromEnv()
	_, ok := r.Get("github")
	if ok {
		t.Error("expected github provider to be absent when secret not set")
	}
}
