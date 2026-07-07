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
