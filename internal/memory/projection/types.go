package projection

import "time"

// Input is one memory entity to project (most-recent observation).
type Input struct {
	EntityID   string
	Content    string    // most-recent observation content (lexical basis + preview)
	Embedding  []float32 // dense vector; nil when the entity has no embedding
	Tier       string    // institutional|agent|user|user_for_agent
	User       string    // virtual_user_id pseudonym; "" for institutional/agent
	Category   string    // consent_category; "" if none
	Title      string
	Confidence float64
	ObservedAt time.Time
}

// Point is one projected memory in the 2D layout.
type Point struct {
	ID         string    `json:"id"`
	X          float64   `json:"x"`
	Y          float64   `json:"y"`
	Tier       string    `json:"tier"`
	User       string    `json:"user,omitempty"`
	Category   string    `json:"category,omitempty"`
	Confidence float64   `json:"confidence"`
	Title      string    `json:"title,omitempty"`
	Preview    string    `json:"preview"`
	ObservedAt time.Time `json:"observedAt"`
}

// Result is the full projection outcome.
type Result struct {
	Model      string  `json:"model"` // "tsne" | "pca"
	Basis      string  `json:"basis"` // "dense" | "lexical"
	Total      int     `json:"total"`
	Unembedded int     `json:"unembedded"`
	Capped     bool    `json:"capped"`
	Points     []Point `json:"points"`
}

const (
	BasisDense   = "dense"
	BasisLexical = "lexical"
	ModelTSNE    = "tsne"
	ModelPCA     = "pca"
)
