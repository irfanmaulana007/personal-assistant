export interface ChatMessage {
  id: string;
  direction: 'in' | 'out';
  body: string;
  timestamp: string;
}

export interface LoginResponse {
  token: string;
  expires_at: number;
}

export interface ChatResponse {
  response: string;
}

export interface HistoryEntry {
  direction: string;
  body: string;
  timestamp: string;
}

export interface LlmProvider {
  id: string;
  label: string;
  default_base_url: string;
  default_model: string;
}

export interface LlmSettings {
  provider: string;
  configured: boolean;
  api_key_mask: string;
  model: string;
  base_url: string;
  providers: LlmProvider[];
}

export interface LlmSettingsUpdate {
  provider?: string;
  api_key?: string;
  model?: string;
  base_url?: string;
}

export interface LlmTestResult {
  ok: boolean;
  model?: string;
  error?: string;
}

export interface UsageSummary {
  requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  estimated_cost_usd: number;
  avg_latency_ms: number;
  tool_calls: number;
}

export interface UsagePlatform {
  platform: string;
  requests: number;
  total_tokens: number;
}

export interface ToolCount {
  tool: string;
  count: number;
}

export interface UsageDay {
  date: string;
  requests: number;
  total_tokens: number;
}

export interface UsageModel {
  model: string;
  requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  estimated_cost_usd: number;
  rate_known: boolean;
}

export interface UsageStats {
  from: string;
  to: string;
  summary: UsageSummary;
  by_day: UsageDay[];
  by_model: UsageModel[];
  by_platform: UsagePlatform[];
  top_tools: ToolCount[];
  cost_partial: boolean;
}
