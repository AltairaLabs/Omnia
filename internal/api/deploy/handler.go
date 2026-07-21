package deploy

import (
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/api/authz"
)

// Handler decodes a DeployIntent and applies it within the caller's workspace
// namespace (resolved by the authz middleware).
type Handler struct {
	applier *Applier
	log     logr.Logger
}

// NewHandler constructs a deploy Handler.
func NewHandler(applier *Applier, log logr.Logger) *Handler {
	return &Handler{applier: applier, log: log}
}

// Deploy handles POST /deployments: decode, validate, apply, respond.
func (h *Handler) Deploy(w http.ResponseWriter, r *http.Request) {
	id, ok := authz.IdentityFromContext(r.Context())
	if !ok || id.Namespace == "" {
		http.Error(w, "missing request identity", http.StatusInternalServerError)
		return
	}

	var intent DeployIntent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIntentBytes)).Decode(&intent); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := intent.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := h.applier.Apply(r.Context(), id.Namespace, intent)

	w.Header().Set("Content-Type", "application/json")
	code := http.StatusOK
	if !result.Succeeded {
		code = http.StatusMultiStatus // 207: applied best-effort, some resources failed
	}
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.log.Error(err, "encode deploy result")
	}
}

// maxIntentBytes bounds the request body (pack.json content can be sizeable but
// not unbounded).
const maxIntentBytes = 4 << 20 // 4 MiB
