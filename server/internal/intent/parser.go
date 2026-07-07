package intent

// Parser defines the interface for intent parsing.
type Parser interface {
	Parse(text string) *ParseResult
}
