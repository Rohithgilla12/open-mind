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

// ParseQuery does no interpretation: the whole query becomes the text portion,
// so FTS-only search keeps working with no AI backend configured.
func (*Noop) ParseQuery(_ context.Context, q string) (ParsedQuery, error) {
	return ParsedQuery{Text: q}, nil
}

func (*Noop) ExtractPlaces(context.Context, string, string) ([]Place, error) {
	return nil, ErrNotSupported
}

func (*Noop) ExtractPlacesVision(context.Context, string, string, []byte) ([]Place, error) {
	return nil, ErrNotSupported
}

func (*Noop) ExtractPlacesVisionFrames(context.Context, string, string, [][]byte) ([]Place, error) {
	return nil, ErrNotSupported
}
