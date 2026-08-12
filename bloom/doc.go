// Package bloom implements process-local probabilistic membership filters.
// Callers must account for false positives and provide external persistence or
// coordination when filters are shared across application instances.
package bloom
