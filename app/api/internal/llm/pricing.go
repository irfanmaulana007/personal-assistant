package llm

import "strings"

// ModelPrice is the USD price per 1,000,000 tokens for a model.
type ModelPrice struct {
	InputPer1M  float64
	OutputPer1M float64
}

// prices holds approximate published rates (USD per 1M tokens) for the default
// model of each built-in provider. These are ESTIMATES for the cost dashboard —
// providers change pricing over time, so treat the dashboard's cost figures as
// approximate and edit this table to match your plan.
var prices = map[string]ModelPrice{
	// DeepSeek rates per https://api-docs.deepseek.com/quick_start/pricing
	// (USD per 1M tokens, cache-miss input). deepseek-chat/reasoner are the
	// non-thinking/thinking aliases of v4-flash and deprecate 2026-07-24.
	"deepseek-v4-flash": {InputPer1M: 0.14, OutputPer1M: 0.28},
	"deepseek-v4-pro":   {InputPer1M: 0.435, OutputPer1M: 0.87},
	"deepseek-chat":     {InputPer1M: 0.14, OutputPer1M: 0.28},
	"deepseek-reasoner": {InputPer1M: 0.14, OutputPer1M: 0.28},
	"gpt-4o-mini":       {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-4o":            {InputPer1M: 2.50, OutputPer1M: 10.00},
	// OpenAI image models (the Image Generator skill) are billed per token, not
	// per image: prompt tokens as input and generated-image tokens as output.
	// gpt-image-1-mini is the low-cost variant the skill uses by default;
	// gpt-image-1 is kept so older traces (and manual overrides) still price.
	// Editing also feeds an input image (billed at a higher input rate by
	// OpenAI); it's folded into the input rate here, so edit costs are a slight
	// underestimate — in line with the rest of this table being approximate.
	// Override via model_prices if you need exact figures.
	"gpt-image-1-mini":        {InputPer1M: 2.50, OutputPer1M: 8.00},
	"gpt-image-1":             {InputPer1M: 5.00, OutputPer1M: 40.00},
	"openai/gpt-4o-mini":      {InputPer1M: 0.15, OutputPer1M: 0.60},
	"llama-3.3-70b-versatile": {InputPer1M: 0.59, OutputPer1M: 0.79},
	"mistral-small-latest":    {InputPer1M: 0.20, OutputPer1M: 0.60},
}

// EstimateCost returns the estimated USD cost for the given token counts and
// whether a rate was known for the model. Unknown models return (0, false).
func EstimateCost(model string, promptTokens, completionTokens int) (float64, bool) {
	p, ok := prices[strings.TrimSpace(model)]
	if !ok {
		return 0, false
	}
	cost := float64(promptTokens)/1_000_000*p.InputPer1M +
		float64(completionTokens)/1_000_000*p.OutputPer1M
	return cost, true
}

// CostFor computes cost from an explicit rate (USD per 1M tokens).
func CostFor(p ModelPrice, promptTokens, completionTokens int) float64 {
	return float64(promptTokens)/1_000_000*p.InputPer1M +
		float64(completionTokens)/1_000_000*p.OutputPer1M
}

// DefaultPrices returns a copy of the built-in rate table (used as fallback
// defaults and shown alongside DB overrides in the UI).
func DefaultPrices() map[string]ModelPrice {
	out := make(map[string]ModelPrice, len(prices))
	for k, v := range prices {
		out[k] = v
	}
	return out
}
