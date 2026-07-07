# Web Search & Weather (Phase 2)

## Overview

Provide web search results and weather information. These capabilities have no external account requirements beyond API keys.

## Weather

### Commands

| Example Messages | Intent |
|-----------------|--------|
| "What's the weather?" | Current weather (default location) |
| "Weather in Tokyo" | Current weather for location |
| "Will it rain tomorrow?" | Forecast |
| "Weather forecast for this week" | Extended forecast |

### Implementation Options

**Option A: wttr.in (Free, no API key)**
```go
func getWeather(location string) (string, error) {
    url := fmt.Sprintf("https://wttr.in/%s?format=j1", url.QueryEscape(location))
    resp, err := http.Get(url)
    // parse JSON response
}
```

**Option B: OpenWeatherMap (Free tier, API key required)**
- 1,000 calls/day free
- More structured data
- Better forecast accuracy

### Response Format
```
Weather in Jakarta:
  Now: 32°C, Partly cloudy
  Humidity: 75%
  Wind: 12 km/h

Today: 28-34°C, Scattered showers in the afternoon
Tomorrow: 27-33°C, Mostly sunny
```

### Configuration
```yaml
weather:
  default_location: "Jakarta"
  units: "metric"  # metric or imperial
  provider: "wttr"  # wttr or openweathermap
  api_key: ""  # only for openweathermap
```

## Web Search

### Commands

| Example Messages | Intent |
|-----------------|--------|
| "Search for Go error handling best practices" | Web search |
| "Google what time is it in New York" | Quick answer |
| "Look up the latest Go release" | Web search |

### Implementation Options

**Option A: SearXNG (Self-hosted, free)**
- Privacy-respecting metasearch engine
- Self-hosted instance
- JSON API available

**Option B: Brave Search API (Free tier)**
- 2,000 queries/month free
- Good result quality

**Option C: Google Custom Search (Free tier)**
- 100 queries/day free
- Most familiar results

### Response Format
```
Search results for "Go error handling best practices":

1. Error handling in Go — go.dev/blog
   "Go's approach to errors is intentionally different..."

2. Best practices for errors in Go — blog.example.com
   "Five patterns every Go developer should know..."

3. Effective Error Handling in Go — medium.com
   "Learn how to wrap, unwrap, and handle errors..."

Reply with a number for more details.
```

## Dependencies

- No Google account required (unlike Calendar/Gmail)
- Weather: either no API key (wttr.in) or free tier key
- Search: depends on chosen provider
