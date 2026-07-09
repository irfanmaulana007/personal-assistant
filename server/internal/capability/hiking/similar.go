package hiking

import "strings"

// normalize lower-cases, trims, and collapses internal whitespace.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// levenshtein returns the edit distance between two strings.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur := make([]int, lb+1)
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(cur[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev = cur
	}
	return prev[lb]
}

// similar reports whether two names likely refer to the same entity — used to
// fold near-duplicate spellings (typos) into an existing canonical name.
func similar(a, b string) bool {
	na, nb := normalize(a), normalize(b)
	if na == nb {
		return true
	}
	if na == "" || nb == "" {
		return false
	}
	d := levenshtein(na, nb)
	if d <= 1 {
		return true
	}
	maxLen := max(len([]rune(na)), len([]rune(nb)))
	return float64(d)/float64(maxLen) <= 0.20
}

// bestMatch returns the candidate most similar to input, or "" if none is close
// enough. Candidates are canonical names already stored.
func bestMatch(candidates []string, input string) string {
	best := ""
	bestDist := 1 << 30
	ni := normalize(input)
	for _, c := range candidates {
		if !similar(c, input) {
			continue
		}
		d := levenshtein(ni, normalize(c))
		if d < bestDist {
			bestDist = d
			best = c
		}
	}
	return best
}
