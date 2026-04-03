package webhook

import (
	"net/http"

	"github.com/ianhomer/cicada/internal/model"
)

// ForgeProvider parses raw HTTP webhook requests into normalized ForgeEvents.
type ForgeProvider interface {
	Name() string
	ParseWebhook(r *http.Request) (*model.ForgeEvent, error)
}
