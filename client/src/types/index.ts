export interface ChatMessage {
  id: string;
  direction: 'in' | 'out';
  body: string;
  timestamp: string;
  image?: string; // data: URL, for user messages with an attached image
  images?: string[]; // data: URLs, for images the assistant generated
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
  images?: string[]; // data: URLs for images the assistant generated this turn
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
  vision: boolean;
  response_mode: string; // 'block' | 'stream'
  providers: LlmProvider[];
}

export interface LlmSettingsUpdate {
  provider?: string;
  api_key?: string;
  model?: string;
  base_url?: string;
  vision?: boolean;
  response_mode?: string; // 'block' | 'stream'
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
  latency_p50_ms: number;
  latency_p95_ms: number;
  latency_p99_ms: number;
  tool_calls: number;
  errors: number;
  active_users: number;
}

export interface UsageUser {
  user_id: number;
  name: string;
  email: string;
  requests: number;
  total_tokens: number;
  errors: number;
  estimated_cost_usd: number;
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
  errors: number;
  total_tokens: number;
  avg_latency_ms: number;
  estimated_cost_usd: number;
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

export interface IntegrationAccount {
  connection_id: string;
  status: IntegrationStatus;
  label?: string; // the account's email, when known
}

export interface IntegrationToolkit {
  slug: string;
  name: string;
  status: IntegrationStatus;
  connection_id?: string;
  multi?: boolean;
  accounts?: IntegrationAccount[];
}

export interface Integrations {
  configured: boolean;
  api_key_mask: string;
  toolkits: IntegrationToolkit[];
  web_search_configured: boolean;
  web_search_key_mask: string;
  openai_configured: boolean;
  openai_key_mask: string;
  trello_configured: boolean;
  trello_key_mask: string;
  trello_token_mask: string;
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
  by_hour: number[];
  by_weekday: number[];
  by_user: UsageUser[];
  cost_partial: boolean;
}

export interface Skill {
  id: number;
  key: string;
  name: string;
  description: string;
  category: string;
  enabled: boolean;
  auto_tuned: boolean; // the end-of-day self-tuner has overridden this skill's prompt
  // Prompt management fields. Present only for admins (the API omits them for
  // members). `prompt_updated_at` is null when the prompt is still the default.
  prompt?: string;
  default_prompt?: string;
  prompt_updated_at?: string | null;
  prompt_updated_by?: string;
}

export interface Preferences {
  timezone: string; // 'UTC' | 'Asia/Jakarta'
  currency: string; // 'USD' | 'IDR'
  usd_to_idr: number;
}

export interface ModelPrice {
  model: string;
  input_per_1m: number;
  output_per_1m: number;
  source: 'custom' | 'builtin';
}

export interface Persona {
  tone: string; // formal | balanced | casual
  emoji: string; // none | occasional | frequent
  length: string; // concise | balanced | detailed
  personality: string; // balanced | professional | friendly | witty | direct | encouraging
  name: string;
  custom: string;
}

export type RepeatMode = 'once' | 'daily' | 'weekly' | 'monthly' | 'specific';

export interface Reminder {
  id: number;
  title: string;
  repeat_mode: RepeatMode;
  times: string[]; // "HH:MM"
  weekdays: number[]; // 0=Sun..6=Sat
  day_of_month: number; // 1-31
  once_date: string; // "YYYY-MM-DD"
  event_at: string; // "YYYY-MM-DDTHH:MM" (specific mode; optional otherwise)
  offsets: number[]; // minutes before event_at (specific mode)
  enabled: boolean;
}

export type ReminderPayload = Omit<Reminder, 'id'>;

export interface RemindersConfig {
  enabled: boolean;
  default_time: string; // 'HH:MM' used when a reminder has no explicit time
}

// Bucket-list category keys, mirrored from the server. The UI maps each to a
// display label; unknown values are stored as 'other'.
export type BucketCategory =
  'self_improvement' | 'learning' | 'hiking' | 'country' | 'local' | 'other';

// A daily routine ("scheduled skill") — a prompt that runs once a day at a set
// time, through the assistant, and delivers its reply over WhatsApp.
export interface Routine {
  key: string;
  name: string;
  description: string;
  enabled: boolean;
  time: string; // local 'HH:MM'
  prompt: string; // effective prompt (override or built-in default)
  default_prompt: string; // built-in default, for a "reset" affordance
  last_run: string; // 'YYYY-MM-DD' of the last fire, or ''
}

export interface RoutineUpdate {
  enabled?: boolean;
  time?: string;
  prompt?: string;
}

export interface BucketItem {
  id: number;
  title: string;
  description: string;
  note: string;
  category: BucketCategory;
  resolution_year: number | null; // set when flagged as that year's resolution
  done: boolean;
  done_at: string; // RFC3339, or '' when not done
  created_at: string;
}

export interface BucketItemPayload {
  title: string;
  description: string;
  note: string;
  category: BucketCategory;
}

/** A single message channel. Filters select any subset — [] means "all". */
export type ChannelValue = 'web' | 'whatsapp';

/** The channels a filter can offer, in display order. */
export const CHANNEL_VALUES: readonly ChannelValue[] = ['web', 'whatsapp'];

export interface ToolInvocation {
  name: string;
  arguments: string;
  result: string;
  latency_ms?: number;
  // Set only for tools that call a paid API of their own (today the Image
  // Generator on gpt-image-1-mini); absent/zero for ordinary tools.
  model?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  estimated_cost_usd?: number;
}

/** Image models the assistant can bill against — used to split LLM vs image
 *  usage on the dashboard. Kept in sync with the server price table. */
export const IMAGE_MODELS: readonly string[] = ['gpt-image-1-mini', 'gpt-image-1'];

export function isImageModel(model: string): boolean {
  return IMAGE_MODELS.includes(model);
}

export interface LLMCall {
  step: number;
  model: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  latency_ms: number;
  finish_reason?: string;
  tool_calls?: string[];
  estimated_cost_usd: number;
}

export interface Trace {
  id: number;
  /** Deployment that served this run (e.g. "local" / "production") — tells you
   *  which database holds the data when debugging a copied run detail. */
  environment?: string;
  user_id: number;
  user?: string;
  platform: string;
  /** What triggered the run: "chat" for an interactive message, or a routine
   *  key ("start_of_day" / "end_of_day") for a scheduled run. */
  source?: string;
  input: string;
  output: string;
  model: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  // Image-generation usage (gpt-image-1-mini), tracked apart from the LLM.
  // combined_total_tokens = LLM + image tokens (shown in the logs list);
  // estimated_cost_usd is the combined LLM+image cost, split into
  // llm_cost_usd / image_cost_usd for the logs detail.
  image_model?: string;
  image_prompt_tokens?: number;
  image_completion_tokens?: number;
  image_total_tokens?: number;
  combined_total_tokens: number;
  latency_ms: number;
  tool_count: number;
  tools?: ToolInvocation[];
  steps?: LLMCall[];
  skills?: string[];
  status: string;
  error?: string;
  estimated_cost_usd: number;
  llm_cost_usd: number;
  image_cost_usd: number;
  score?: TraceScore;
  created_at: string;
}

/** LLM-as-judge quality verdict for a trace (each dimension 1–5). */
export interface TraceScore {
  accuracy: number;
  helpfulness: number;
  safety: number;
  overall: number;
  rationale?: string;
  judge_model?: string;
}

/** A judge-score bucket for the logs list. Filters select any subset — [] = all. */
export type ScoreValue = 'scored' | 'unscored' | 'low';

/** The score buckets a filter can offer, in display order. */
export const SCORE_VALUES: readonly ScoreValue[] = ['scored', 'unscored', 'low'];

export interface LogsResponse {
  traces: Trace[];
  next_cursor?: number;
}
