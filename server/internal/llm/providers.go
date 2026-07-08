package llm

// ProviderInfo describes a selectable LLM provider and its preset defaults.
// All listed providers speak the OpenAI-compatible chat-completions + tool
// calling API, so a single Client handles them; only the base URL and default
// model differ. "custom" lets the user point at any other compatible endpoint.
type ProviderInfo struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	DefaultBaseURL string `json:"default_base_url"`
	DefaultModel   string `json:"default_model"`
}

// DefaultProvider is used when none has been configured.
const DefaultProvider = "deepseek"

// Providers is the registry of known providers, in display order.
var Providers = []ProviderInfo{
	{ID: "deepseek", Label: "DeepSeek", DefaultBaseURL: "https://api.deepseek.com", DefaultModel: "deepseek-chat"},
	{ID: "openai", Label: "OpenAI", DefaultBaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-4o-mini"},
	{ID: "openrouter", Label: "OpenRouter", DefaultBaseURL: "https://openrouter.ai/api/v1", DefaultModel: "openai/gpt-4o-mini"},
	{ID: "groq", Label: "Groq", DefaultBaseURL: "https://api.groq.com/openai/v1", DefaultModel: "llama-3.3-70b-versatile"},
	{ID: "mistral", Label: "Mistral", DefaultBaseURL: "https://api.mistral.ai/v1", DefaultModel: "mistral-small-latest"},
	{ID: "custom", Label: "Custom (OpenAI-compatible)", DefaultBaseURL: "", DefaultModel: ""},
}

// ProviderByID returns the provider preset for id, if known.
func ProviderByID(id string) (ProviderInfo, bool) {
	for _, p := range Providers {
		if p.ID == id {
			return p, true
		}
	}
	return ProviderInfo{}, false
}
