package store

import (
	"strings"
	"unicode"
)

// sqliteFTS5Query turns arbitrary user text into a safe SQLite FTS5 MATCH query:
// lowercase alphanumeric tokens (len >= 3), deduped, quoted, and OR-joined — so
// raw input can't break FTS5 syntax and recall stays broad. Returns "" when the
// text yields no usable terms, which callers treat as "no match".
//
// This lives in the store layer (not the service layer) on purpose: each backend
// sanitizes for its own query dialect. The SQLite backend uses this FTS5 form;
// the PostgreSQL backend will hand raw text to websearch_to_tsquery instead.
// Services therefore pass raw text and let the backend decide.
func sqliteFTS5Query(text string) string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	seen := make(map[string]bool)
	terms := make([]string, 0, 12)
	for _, f := range fields {
		if len([]rune(f)) < 3 || seen[f] {
			continue
		}
		seen[f] = true
		terms = append(terms, `"`+f+`"`)
		if len(terms) >= 12 {
			break
		}
	}
	return strings.Join(terms, " OR ")
}
