import type {
  AuthResponse,
  User,
  MyStats,
  Preferences,
  Persona,
  Reminder,
  ReminderPayload,
  RemindersConfig,
  ModelPrice,
  LifeGoal,
  LifeGoalPayload,
  Skill,
  Role,
  ChatResponse,
  HistoryEntry,
  LlmSettings,
  LlmSettingsUpdate,
  LlmTestResult,
  UsageStats,
  Integrations,
  WhatsAppStatus,
  Channel,
  LogsResponse,
  Trace,
} from '../types';

const TOKEN_KEY = 'assistant_token';

function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export function isAuthenticated(): boolean {
  return getToken() !== null;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...((options.headers as Record<string, string>) || {}),
  };

  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(path, { ...options, headers });

  if (res.status === 401) {
    clearToken();
    window.location.reload();
    throw new Error('Unauthorized');
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `Request failed: ${res.status}`);
  }

  return res.json();
}

export async function getAuthStatus(): Promise<{ setup_required: boolean }> {
  return request<{ setup_required: boolean }>('/api/auth/status');
}

export async function login(email: string, password: string): Promise<AuthResponse> {
  const data = await request<AuthResponse>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
  setToken(data.token);
  return data;
}

export async function setupAdmin(email: string, password: string): Promise<AuthResponse> {
  const data = await request<AuthResponse>('/api/auth/setup', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
  setToken(data.token);
  return data;
}

export async function getMe(): Promise<User> {
  return request<User>('/api/auth/me');
}

export async function updateProfile(name: string, email: string): Promise<User> {
  return request<User>('/api/auth/me', {
    method: 'PATCH',
    body: JSON.stringify({ name, email }),
  });
}

export async function getMyStats(): Promise<MyStats> {
  return request<MyStats>('/api/auth/me/stats');
}

export async function getPreferences(): Promise<Preferences> {
  return request<Preferences>('/api/preferences');
}

export async function updatePreferences(p: Preferences): Promise<Preferences> {
  return request<Preferences>('/api/preferences', { method: 'PUT', body: JSON.stringify(p) });
}

export async function getPersona(): Promise<Persona> {
  return request<Persona>('/api/persona');
}

export async function updatePersona(p: Persona): Promise<Persona> {
  return request<Persona>('/api/persona', { method: 'PUT', body: JSON.stringify(p) });
}

export async function listReminders(): Promise<Reminder[]> {
  return request<Reminder[]>('/api/reminders');
}

export async function createReminder(r: ReminderPayload): Promise<Reminder> {
  return request<Reminder>('/api/reminders', { method: 'POST', body: JSON.stringify(r) });
}

export async function updateReminder(id: number, r: ReminderPayload): Promise<Reminder> {
  return request<Reminder>(`/api/reminders/${id}`, { method: 'PUT', body: JSON.stringify(r) });
}

export async function setReminderEnabled(id: number, enabled: boolean): Promise<Reminder> {
  return request<Reminder>(`/api/reminders/${id}/enabled`, {
    method: 'PUT',
    body: JSON.stringify({ enabled }),
  });
}

export async function deleteReminder(id: number): Promise<void> {
  await request(`/api/reminders/${id}`, { method: 'DELETE' });
}

export async function getRemindersConfig(): Promise<RemindersConfig> {
  return request<RemindersConfig>('/api/reminders/config');
}

export async function setRemindersConfig(cfg: RemindersConfig): Promise<RemindersConfig> {
  return request<RemindersConfig>('/api/reminders/config', {
    method: 'PUT',
    body: JSON.stringify(cfg),
  });
}

export async function listLifeGoals(): Promise<LifeGoal[]> {
  return request<LifeGoal[]>('/api/life-goals');
}

export async function createLifeGoal(g: LifeGoalPayload): Promise<LifeGoal> {
  return request<LifeGoal>('/api/life-goals', { method: 'POST', body: JSON.stringify(g) });
}

export async function updateLifeGoal(id: number, g: LifeGoalPayload): Promise<LifeGoal> {
  return request<LifeGoal>(`/api/life-goals/${id}`, { method: 'PUT', body: JSON.stringify(g) });
}

export async function setLifeGoalDone(id: number, done: boolean): Promise<LifeGoal> {
  return request<LifeGoal>(`/api/life-goals/${id}/done`, {
    method: 'PUT',
    body: JSON.stringify({ done }),
  });
}

export async function deleteLifeGoal(id: number): Promise<void> {
  await request(`/api/life-goals/${id}`, { method: 'DELETE' });
}

export async function getPricing(): Promise<ModelPrice[]> {
  return request<ModelPrice[]>('/api/pricing');
}

export async function setPricing(
  model: string,
  inputPer1M: number,
  outputPer1M: number,
): Promise<ModelPrice[]> {
  return request<ModelPrice[]>('/api/pricing', {
    method: 'PUT',
    body: JSON.stringify({ model, input_per_1m: inputPer1M, output_per_1m: outputPer1M }),
  });
}

export async function deletePricing(model: string): Promise<ModelPrice[]> {
  return request<ModelPrice[]>(`/api/pricing/${encodeURIComponent(model)}`, { method: 'DELETE' });
}

export async function listSkills(): Promise<Skill[]> {
  return request<Skill[]>('/api/skills');
}

export async function setSkillEnabled(id: number, enabled: boolean): Promise<Skill[]> {
  return request<Skill[]>(`/api/skills/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ enabled }),
  });
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await request('/api/auth/password', {
    method: 'POST',
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
}

export async function listUsers(): Promise<User[]> {
  return request<User[]>('/api/users');
}

export async function createUser(email: string, password: string, role: Role): Promise<User> {
  return request<User>('/api/users', {
    method: 'POST',
    body: JSON.stringify({ email, password, role }),
  });
}

export async function updateUser(
  id: number,
  changes: { role?: Role; password?: string },
): Promise<User> {
  return request<User>(`/api/users/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(changes),
  });
}

export async function deleteUser(id: number): Promise<void> {
  await request(`/api/users/${id}`, { method: 'DELETE' });
}

export async function sendMessage(message: string, image?: string): Promise<ChatResponse> {
  return request<ChatResponse>('/api/chat', {
    method: 'POST',
    body: JSON.stringify({ message, image: image ?? '' }),
  });
}

export async function getChatHistory(): Promise<HistoryEntry[]> {
  return request<HistoryEntry[]>('/api/chat/history');
}

export async function getSettings(): Promise<LlmSettings> {
  return request<LlmSettings>('/api/settings');
}

export async function updateSettings(update: LlmSettingsUpdate): Promise<LlmSettings> {
  return request<LlmSettings>('/api/settings', {
    method: 'PUT',
    body: JSON.stringify(update),
  });
}

export async function testSettings(): Promise<LlmTestResult> {
  return request<LlmTestResult>('/api/settings/test', { method: 'POST' });
}

export async function getUsage(
  from: string,
  to: string,
  platform: Channel = '',
): Promise<UsageStats> {
  const p = platform ? `&platform=${platform}` : '';
  return request<UsageStats>(`/api/metrics/usage?from=${from}&to=${to}${p}`);
}

export async function getLogs(
  from: string,
  to: string,
  platform: Channel = '',
  limit = 25,
  cursor = 0,
): Promise<LogsResponse> {
  const p = platform ? `&platform=${platform}` : '';
  const c = cursor ? `&cursor=${cursor}` : '';
  return request<LogsResponse>(`/api/logs?from=${from}&to=${to}&limit=${limit}${p}${c}`);
}

export async function getLog(id: number): Promise<Trace> {
  return request<Trace>(`/api/logs/${id}`);
}

export async function getIntegrations(): Promise<Integrations> {
  return request<Integrations>('/api/integrations');
}

export async function setComposioKey(apiKey: string): Promise<Integrations> {
  return request<Integrations>('/api/integrations/key', {
    method: 'PUT',
    body: JSON.stringify({ api_key: apiKey }),
  });
}

export async function connectIntegration(slug: string): Promise<{ redirect_url: string }> {
  return request<{ redirect_url: string }>(`/api/integrations/${slug}/connect`, { method: 'POST' });
}

export async function disconnectIntegration(
  slug: string,
  connectionId?: string,
): Promise<Integrations> {
  const q = connectionId ? `?connection_id=${encodeURIComponent(connectionId)}` : '';
  return request<Integrations>(`/api/integrations/${slug}${q}`, { method: 'DELETE' });
}

export async function getWhatsApp(): Promise<WhatsAppStatus> {
  return request<WhatsAppStatus>('/api/whatsapp');
}

export async function connectWhatsApp(): Promise<WhatsAppStatus> {
  return request<WhatsAppStatus>('/api/whatsapp/connect', { method: 'POST' });
}

export async function disconnectWhatsApp(): Promise<WhatsAppStatus> {
  return request<WhatsAppStatus>('/api/whatsapp/disconnect', { method: 'POST' });
}

export async function getWhatsAppAllowlist(): Promise<{ allowlist: string[] }> {
  return request<{ allowlist: string[] }>('/api/whatsapp/allowlist');
}

export async function setWhatsAppAllowlist(allowlist: string[]): Promise<{ allowlist: string[] }> {
  return request<{ allowlist: string[] }>('/api/whatsapp/allowlist', {
    method: 'PUT',
    body: JSON.stringify({ allowlist }),
  });
}
