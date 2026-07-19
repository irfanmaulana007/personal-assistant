import type {
  AuthResponse,
  User,
  MyStats,
  Preferences,
  Persona,
  Reminder,
  ReminderPayload,
  RemindersConfig,
  Routine,
  RoutineUpdate,
  ModelPrice,
  BucketItem,
  BucketItemPayload,
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
  ChannelValue,
  LogsResponse,
  ScoreValue,
  Trace,
} from '../types';

const TOKEN_KEY = 'assistant_token';
const PROJECT_KEY = 'assistant_project';

function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

// The active project id is sent on every request as X-Project-Id so the server
// scopes domain data, skills, and chat to it. It is persisted so a reload keeps
// the same active project.
export function getActiveProjectId(): number | null {
  const v = localStorage.getItem(PROJECT_KEY);
  return v ? Number(v) : null;
}

export function setActiveProjectId(id: number | null): void {
  if (id == null) {
    localStorage.removeItem(PROJECT_KEY);
  } else {
    localStorage.setItem(PROJECT_KEY, String(id));
  }
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

  const projectId = getActiveProjectId();
  if (projectId && !headers['X-Project-Id']) {
    headers['X-Project-Id'] = String(projectId);
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

// Deletes every event on the user's Composio-connected Google Calendar(s).
// Destructive recovery action for wiping a flood of duplicate events; returns
// how many were deleted and how many delete calls failed.
export async function clearAllCalendarEvents(): Promise<{ deleted: number; failed: number }> {
  return request<{ deleted: number; failed: number }>('/api/calendar/events', {
    method: 'DELETE',
  });
}

export async function getRoutines(): Promise<Routine[]> {
  return request<Routine[]>('/api/routines');
}

export async function updateRoutine(key: string, update: RoutineUpdate): Promise<Routine> {
  return request<Routine>(`/api/routines/${key}`, {
    method: 'PUT',
    body: JSON.stringify(update),
  });
}

export async function runRoutine(key: string): Promise<{ sent: boolean; message: string }> {
  return request<{ sent: boolean; message: string }>(`/api/routines/${key}/run`, {
    method: 'POST',
  });
}

export async function listBucketItems(): Promise<BucketItem[]> {
  return request<BucketItem[]>('/api/bucket-list');
}

export async function createBucketItem(g: BucketItemPayload): Promise<BucketItem> {
  return request<BucketItem>('/api/bucket-list', { method: 'POST', body: JSON.stringify(g) });
}

export async function updateBucketItem(id: number, g: BucketItemPayload): Promise<BucketItem> {
  return request<BucketItem>(`/api/bucket-list/${id}`, { method: 'PUT', body: JSON.stringify(g) });
}

// Mark an item done/undone. When checking, an optional `doneAt` (RFC3339 or
// "YYYY-MM-DD") records the exact completion date; omit it to default to now.
export async function setBucketItemDone(
  id: number,
  done: boolean,
  doneAt?: string,
): Promise<BucketItem> {
  return request<BucketItem>(`/api/bucket-list/${id}/done`, {
    method: 'PUT',
    body: JSON.stringify({ done, done_at: done ? doneAt : undefined }),
  });
}

export async function setBucketItemResolution(
  id: number,
  year: number | null,
): Promise<BucketItem> {
  return request<BucketItem>(`/api/bucket-list/${id}/resolution`, {
    method: 'PUT',
    body: JSON.stringify({ year }),
  });
}

export async function deleteBucketItem(id: number): Promise<void> {
  await request(`/api/bucket-list/${id}`, { method: 'DELETE' });
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

// Admin only. Saves a custom prompt for a skill; returns the refreshed list.
export async function setSkillPrompt(id: number, prompt: string): Promise<Skill[]> {
  return request<Skill[]>(`/api/skills/${id}/prompt`, {
    method: 'PUT',
    body: JSON.stringify({ prompt }),
  });
}

// Resets a skill's prompt back to the shipped default. For a global skill this
// is superadmin-only and hands the prompt back to the boot seed; for a project
// fork it restores the base default while keeping the fork.
export async function resetSkillPrompt(id: number): Promise<Skill[]> {
  return request<Skill[]>(`/api/skills/${id}/prompt`, {
    method: 'PUT',
    body: JSON.stringify({ reset: true }),
  });
}

// Project admin only. Forks a global skill into the active project so it can be
// given a project-specific prompt; returns the refreshed list.
export async function customizeSkill(id: number): Promise<Skill[]> {
  return request<Skill[]>(`/api/skills/${id}/customize`, { method: 'POST' });
}

// Project admin only. Removes the active project's fork of a skill, reverting it
// to the shared global skill; returns the refreshed list.
export async function deleteProjectSkill(id: number): Promise<Skill[]> {
  return request<Skill[]>(`/api/skills/${id}`, { method: 'DELETE' });
}

// clearTunedPrompt clears a skill's auto-tuned prompt override (set by the
// end-of-day self-tuner), reverting it to the shipped default. Admin-only.
export async function clearTunedPrompt(id: number): Promise<Skill[]> {
  return request<Skill[]>(`/api/skills/${id}/reset-prompt`, { method: 'POST' });
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

export interface ChatStreamHandlers {
  // onDelta receives the full accumulated reply text so far (not just the latest
  // fragment), so a caller can render it by simply replacing the message body.
  onDelta?: (text: string) => void;
}

// sendMessage posts a chat message and returns the assistant's reply. Delivery
// is decided by the server's admin "response mode": in block mode it replies
// with a single JSON body; in stream mode it replies with Server-Sent Events,
// which this function consumes, invoking handlers.onDelta as text arrives. The
// final ChatResponse is returned identically either way, so callers that don't
// pass handlers still work unchanged.
export async function sendMessage(
  message: string,
  image?: string,
  handlers?: ChatStreamHandlers,
): Promise<ChatResponse> {
  const token = getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch('/api/chat', {
    method: 'POST',
    headers,
    body: JSON.stringify({ message, image: image ?? '' }),
  });

  if (res.status === 401) {
    clearToken();
    window.location.reload();
    throw new Error('Unauthorized');
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `Request failed: ${res.status}`);
  }

  const contentType = res.headers.get('content-type') || '';
  if (!contentType.includes('text/event-stream') || !res.body) {
    return res.json();
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let response = '';
  let images: string[] | undefined;
  let errored: string | null = null;

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    // SSE frames are separated by a blank line.
    let sep: number;
    while ((sep = buffer.indexOf('\n\n')) !== -1) {
      const frame = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);
      const dataLine = frame.split('\n').find((l) => l.startsWith('data:'));
      if (!dataLine) continue;
      const payload = dataLine.slice(5).trim();
      if (!payload) continue;
      let evt: {
        type?: string;
        text?: string;
        response?: string;
        images?: string[];
        error?: string;
      };
      try {
        evt = JSON.parse(payload);
      } catch {
        continue;
      }
      if (evt.type === 'delta') {
        response += evt.text ?? '';
        handlers?.onDelta?.(response);
      } else if (evt.type === 'done') {
        if (typeof evt.response === 'string') response = evt.response;
        images = evt.images;
        handlers?.onDelta?.(response);
      } else if (evt.type === 'error') {
        errored = evt.error ?? 'Streaming failed';
      }
    }
  }

  if (errored) throw new Error(errored);
  return { response, images };
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

// Multi-value filters (platform, score) are sent as a single comma-separated
// query param; an empty array omits the param entirely ("all").
//
// projectId overrides the active-project scope for this one request (via the
// X-Project-Id header, which request() leaves alone when already set). A
// superadmin uses this to read any project's usage without switching projects.
export async function getUsage(
  from: string,
  to: string,
  platforms: ChannelValue[] = [],
  projectId?: number,
): Promise<UsageStats> {
  const p = platforms.length ? `&platform=${platforms.join(',')}` : '';
  const options = projectId ? { headers: { 'X-Project-Id': String(projectId) } } : {};
  return request<UsageStats>(`/api/metrics/usage?from=${from}&to=${to}${p}`, options);
}

export async function getLogs(
  from: string,
  to: string,
  platforms: ChannelValue[] = [],
  limit = 25,
  cursor = 0,
  scores: ScoreValue[] = [],
): Promise<LogsResponse> {
  const p = platforms.length ? `&platform=${platforms.join(',')}` : '';
  const c = cursor ? `&cursor=${cursor}` : '';
  const s = scores.length ? `&score=${scores.join(',')}` : '';
  return request<LogsResponse>(`/api/logs?from=${from}&to=${to}&limit=${limit}${p}${c}${s}`);
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

export async function setWebSearchKey(apiKey: string): Promise<Integrations> {
  return request<Integrations>('/api/integrations/websearch-key', {
    method: 'PUT',
    body: JSON.stringify({ api_key: apiKey }),
  });
}

export async function setOpenAIKey(apiKey: string): Promise<Integrations> {
  return request<Integrations>('/api/integrations/openai-key', {
    method: 'PUT',
    body: JSON.stringify({ api_key: apiKey }),
  });
}

export async function setTrelloCreds(apiKey: string, token: string): Promise<Integrations> {
  return request<Integrations>('/api/integrations/trello-creds', {
    method: 'PUT',
    body: JSON.stringify({ api_key: apiKey, token }),
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

export interface WhatsAppAllowlist {
  allowlist: string[];
  allow_all: boolean;
}

export async function getWhatsAppAllowlist(): Promise<WhatsAppAllowlist> {
  return request<WhatsAppAllowlist>('/api/whatsapp/allowlist');
}

export async function setWhatsAppAllowlist(
  allowlist: string[],
  allowAll: boolean,
): Promise<WhatsAppAllowlist> {
  return request<WhatsAppAllowlist>('/api/whatsapp/allowlist', {
    method: 'PUT',
    body: JSON.stringify({ allowlist, allow_all: allowAll }),
  });
}

// --- Projects & RBAC ---

export async function listProjects(): Promise<import('../types').Project[]> {
  return request('/api/projects');
}

export async function createProject(
  name: string,
  adminEmail?: string,
): Promise<import('../types').Project> {
  return request('/api/projects', {
    method: 'POST',
    body: JSON.stringify({ name, admin_email: adminEmail || '' }),
  });
}

export async function updateProject(id: number, name: string): Promise<{ ok: boolean }> {
  return request(`/api/projects/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) });
}

export async function deleteProject(id: number): Promise<{ ok: boolean }> {
  return request(`/api/projects/${id}`, { method: 'DELETE' });
}

export async function listProjectMembers(id: number): Promise<import('../types').ProjectMember[]> {
  return request(`/api/projects/${id}/members`);
}

export async function addProjectMember(
  id: number,
  email: string,
  role: 'admin' | 'member',
): Promise<import('../types').ProjectMember[]> {
  return request(`/api/projects/${id}/members`, {
    method: 'POST',
    body: JSON.stringify({ email, role }),
  });
}

// createProjectMember creates a brand-new user account and adds them to the
// project in a single request (for onboarding someone who has no account yet).
// Returns the refreshed member list, like addProjectMember.
export async function createProjectMember(
  id: number,
  email: string,
  password: string,
  role: 'admin' | 'member',
): Promise<import('../types').ProjectMember[]> {
  return request(`/api/projects/${id}/members/create`, {
    method: 'POST',
    body: JSON.stringify({ email, password, role }),
  });
}

export async function updateProjectMember(
  id: number,
  userId: number,
  role: 'admin' | 'member',
): Promise<import('../types').ProjectMember[]> {
  return request(`/api/projects/${id}/members/${userId}`, {
    method: 'PATCH',
    body: JSON.stringify({ role }),
  });
}

export async function removeProjectMember(
  id: number,
  userId: number,
): Promise<import('../types').ProjectMember[]> {
  return request(`/api/projects/${id}/members/${userId}`, { method: 'DELETE' });
}

export async function listProjectAudit(id: number): Promise<import('../types').AuditEvent[]> {
  return request(`/api/projects/${id}/audit`);
}

// Features are scoped to the active project via the X-Project-Id header.
export async function listFeatures(): Promise<import('../types').ProjectFeature[]> {
  return request('/api/features');
}

export async function setFeatureEnabled(
  id: number,
  enabled: boolean,
): Promise<import('../types').ProjectFeature[]> {
  return request(`/api/features/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ enabled }),
  });
}

// --- WhatsApp mappings (superadmin) ---

export async function listWhatsAppMappings(): Promise<import('../types').WhatsAppMapping[]> {
  return request('/api/whatsapp/mappings');
}

export async function createWhatsAppMapping(
  m: Omit<import('../types').WhatsAppMapping, 'id' | 'created_at'>,
): Promise<import('../types').WhatsAppMapping> {
  return request('/api/whatsapp/mappings', { method: 'POST', body: JSON.stringify(m) });
}

export async function updateWhatsAppMapping(
  id: number,
  m: Omit<import('../types').WhatsAppMapping, 'id' | 'created_at'>,
): Promise<{ ok: boolean }> {
  return request(`/api/whatsapp/mappings/${id}`, { method: 'PATCH', body: JSON.stringify(m) });
}

export async function deleteWhatsAppMapping(id: number): Promise<{ ok: boolean }> {
  return request(`/api/whatsapp/mappings/${id}`, { method: 'DELETE' });
}

// --- Superadmin cross-project overview ---

export async function getAdminOverview(
  from?: string,
  to?: string,
  projectId?: number,
): Promise<import('../types').AdminOverview> {
  const p = new URLSearchParams();
  if (from) p.set('from', from);
  if (to) p.set('to', to);
  if (projectId) p.set('projectId', String(projectId));
  const qs = p.toString();
  return request(`/api/admin/overview${qs ? `?${qs}` : ''}`);
}

// --- Per-project skills & features (path-scoped: manage a specific project) ---

export async function getProjectSkills(id: number): Promise<import('../types').ProjectSkill[]> {
  return request(`/api/projects/${id}/skills`);
}

export async function setProjectSkill(
  id: number,
  skillId: number,
  enabled: boolean,
): Promise<import('../types').ProjectSkill[]> {
  return request(`/api/projects/${id}/skills/${skillId}`, {
    method: 'PUT',
    body: JSON.stringify({ enabled }),
  });
}

export async function getProjectFeatures(id: number): Promise<import('../types').ProjectFeature[]> {
  return request(`/api/projects/${id}/features`);
}

export async function setProjectFeature(
  id: number,
  featureId: number,
  enabled: boolean,
): Promise<import('../types').ProjectFeature[]> {
  return request(`/api/projects/${id}/features/${featureId}`, {
    method: 'PUT',
    body: JSON.stringify({ enabled }),
  });
}
