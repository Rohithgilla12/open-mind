package ai

import "context"

// Noop is a Provider that performs no AI work. It is the default provider
// and keeps the app fully functional with manual tags and FTS-only search.
type Noop struct{}

// NewNoop returns a new noop Provider.
func NewNoop() *Noop { return &Noop{} }

func (*Noop) Name() string { return "noop" }

func (*Noop) Summarise(context.Context, string, string) (string, error) { return "", nil }

func (*Noop) Tag(context.Context, string, string) ([]string, error) { return nil, nil }

func (*Noop) Embed(context.Context, string) ([]float32, error) { return nil, ErrNotSupported }

func (*Noop) ParseQuery(_ context.Context, q string) (string, error) { return q, nil }
