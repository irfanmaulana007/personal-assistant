export interface ChatMessage {
  id: string;
  direction: 'in' | 'out';
  body: string;
  timestamp: string;
}

export type Role = 'admin' | 'member';

export interface User {
  id: number;
  email: string;
  name: string;
  role: Role;
  created_at: string;
}

export interface AuthResponse {
  token: string;
  expires_at: number;
  user: User;
}

export interface MyStats {
  runs: number;
  total_tokens: number;
  reminders: number;
  notes: number;
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
  errors: number;
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

export type IntegrationStatus = 'connected' | 'pending' | 'error' | 'disconnected';

export interface IntegrationToolkit {
  slug: string;
  name: string;
  status: IntegrationStatus;
  connection_id?: string;
}

export interface Integrations {
  configured: boolean;
  api_key_mask: string;
  toolkits: IntegrationToolkit[];
}

export type WhatsAppState = 'disconnected' | 'pairing' | 'connected' | 'disabled';

export interface WhatsAppStatus {
  enabled: boolean;
  status: WhatsAppState;
  qr: string; // data:image/png;base64,... while pairing
}

export interface UsageStats {
  from: string;
  to: string;
  platform: string;
  summary: UsageSummary;
  by_day: UsageDay[];
  by_model: UsageModel[];
  by_platform: UsagePlatform[];
  top_tools: ToolCount[];
  cost_partial: boolean;
}

export type Channel = '' | 'web' | 'whatsapp';

export interface ToolInvocation {
  name: string;
  arguments: string;
  result: string;
}

export interface Trace {
  id: number;
  platform: string;
  input: string;
  output: string;
  model: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  latency_ms: number;
  tool_count: number;
  tools?: ToolInvocation[];
  status: string;
  error?: string;
  estimated_cost_usd: number;
  created_at: string;
}

export interface LogsResponse {
  traces: Trace[];
}
