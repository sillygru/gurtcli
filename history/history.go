package history

const maxEntries = 1000

// Add appends input to the in-memory history slice, deduplicating against the
// last entry. The returned slice is capped at maxEntries.
func Add(hist []string, input string) []string {
	if len(hist) > 0 && hist[len(hist)-1] == input {
		return hist
	}
	hist = append(hist, input)
	if len(hist) > maxEntries {
		hist = hist[len(hist)-maxEntries:]
	}
	return hist
}
