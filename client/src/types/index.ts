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
