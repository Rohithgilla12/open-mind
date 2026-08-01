package api

// SetRaindropBaseForTest points the Raindrop.io import at a fake server and
// returns a func restoring the real origin. Test-only seam: the base URL is a
// package var precisely so the handler never needs config plumbing for a host
// that only ever changes under test.
func SetRaindropBaseForTest(base string) func() {
	old := raindropBase
	raindropBase = base
	return func() { raindropBase = old }
}
