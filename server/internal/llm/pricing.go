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
	"deepseek-chat":           {InputPer1M: 0.27, OutputPer1M: 1.10},
	"deepseek-reasoner":       {InputPer1M: 0.55, OutputPer1M: 2.19},
	"gpt-4o-mini":             {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-4o":                  {InputPer1M: 2.50, OutputPer1M: 10.00},
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
