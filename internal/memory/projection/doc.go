// Package projection turns a set of memory entities into a stable 2D layout
// for the Memory Galaxy view. It is pure: no DB or HTTP. The pipeline is
// basis-selection -> vectorize -> reduce -> t-SNE -> Procrustes-align to the
// previous layout -> normalize.
package projection
