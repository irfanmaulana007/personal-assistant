// Derives a friendly, present-tense loading phrase from the user's message so
// the chat "typing" bubble reads like "Checking your calendar..." instead of a
// generic animation. The chat endpoint is a single non-streaming request, so we
// have no live tool-step signal — we infer intent from the input keywords,
// grounded in the agent's actual tool set (see server/internal/agent/tools.go).

interface PhraseRule {
  phrase: string;
  keywords: string[];
}

// Order matters: the first rule whose keyword appears in the message wins, so
// more specific intents are listed before broader ones.
const RULES: PhraseRule[] = [
  {
    phrase: 'Searching the web',
    keywords: [
      'weather',
      'news',
      'price',
      'stock',
      'latest',
      'google',
      'search the web',
      'web search',
      'online',
    ],
  },
  {
    phrase: 'Working on your image',
    keywords: [
      'image',
      'photo',
      'picture',
      'draw',
      'generate',
      'edit this',
      'logo',
      'illustration',
    ],
  },
  {
    phrase: 'Checking your calendar',
    keywords: [
      'calendar',
      'schedule',
      'meeting',
      'appointment',
      'event',
      'agenda',
      'free time',
      'availability',
    ],
  },
  {
    phrase: 'Checking your email',
    keywords: ['email', 'inbox', 'mail', 'message from', 'unread', 'draft'],
  },
  {
    phrase: 'Looking through your notes',
    keywords: ['note', 'notes'],
  },
  {
    phrase: 'Setting up your reminder',
    keywords: ['remind', 'reminder', 'alert me', 'notify me'],
  },
  {
    phrase: 'Searching my memory',
    keywords: ['remember', 'recall', 'do you know', 'what do you know', 'my preference'],
  },
  {
    phrase: 'Looking up your contacts',
    keywords: ['contact', 'phone number', 'who is'],
  },
  {
    phrase: 'Checking your bucket list',
    keywords: ['bucket list', 'bucketlist'],
  },
  {
    phrase: 'Reviewing your activity',
    keywords: ['workout', 'exercise', 'run ', 'ran ', 'gym', 'sport', 'activity', 'training'],
  },
  {
    phrase: 'Working on your trip',
    keywords: ['trip', 'expense', 'travel', 'budget', 'spending'],
  },
  {
    phrase: 'Checking your hikes',
    keywords: ['hike', 'hiking', 'mountain', 'trail'],
  },
];

const DEFAULT_PHRASE = 'Thinking';

/**
 * Returns a present-tense loading phrase (without a trailing ellipsis) for the
 * given user message, e.g. "Checking your calendar". Falls back to "Thinking".
 */
export function loadingPhrase(text: string | undefined): string {
  if (!text) return DEFAULT_PHRASE;
  const lower = ` ${text.toLowerCase()} `;
  for (const rule of RULES) {
    if (rule.keywords.some((kw) => lower.includes(kw))) {
      return rule.phrase;
    }
  }
  return DEFAULT_PHRASE;
}
