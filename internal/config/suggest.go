package config

import (
	"fmt"
	"slices"
	"strings"
)

// suggestion returns ` (did you mean "x"?)` for the closest candidate, or ""
// when nothing is close enough. Ties are broken alphabetically so output is
// deterministic.
func suggestion(got string, candidates []string) string {
	best, bestDist := "", -1
	sorted := slices.Clone(candidates)
	slices.Sort(sorted)
	lg := strings.ToLower(got)
	for _, c := range sorted {
		lc := strings.ToLower(c)
		dist := levenshtein(lg, lc)
		isPrefix := len(lg) >= 4 && (strings.HasPrefix(lc, lg) || strings.HasPrefix(lg, lc))
		if dist > max(2, len(lc)/3) && !isPrefix {
			continue
		}
		if bestDist < 0 || dist < bestDist {
			best, bestDist = c, dist
		}
	}
	if best == "" || best == got {
		return ""
	}
	return fmt.Sprintf(" (did you mean %q?)", best)
}

// levenshtein returns the edit distance between a and b, in runes.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
