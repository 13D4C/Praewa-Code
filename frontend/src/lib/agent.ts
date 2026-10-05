import { isDesktop, onEvent } from './bridge';
import type { AgentMode } from './agent-modes';

export type AgentConnection = {
  connected: boolean;
  error: string;
};
export type AgentThread = {
  id: string;
  name: string;
  preview: string;
  cwd: string;
  model: string;
  updatedAt: number;
  status: { type: string };
};
export type AgentMessage = {
  id: string;
  turnId: string;
  role: 'user' | 'assistant';
  text: string;
  phase: string;
  status: string;
  createdAt: number;
};
export type AgentRequest = {
  id: string;
  threadId: string;
  turnId: string;
  kind: 'approval' | 'question';
  title: string;
  detail: string;
  questions: {
    id: string;
    question: string;
    options: { label: string; description: string }[];
  }[];
};
export type AgentConversation = {
  thread: AgentThread;
  sequence: number;
  messages: AgentMessage[];
  requests: AgentRequest[];
  turnId: string;
  active: boolean;
  canStop: boolean;
  error: string;
};
export type AgentSendRequest = {
  id: string;
  threadId: string;
  text: string;
  mode: AgentMode;
  files: { name: string; path: string; content: string }[];
};
export type AgentAnswer = {
  threadId: string;
  id: string;
  accept: boolean;
  answers: Record<string, string>;
};
type AgentBackend = {
  AgentConnect(): Promise<AgentConnection>;
  AgentStatus(): Promise<AgentConnection>;
  AgentThreads(
    cursor: string,
  ): Promise<{ data: AgentThread[]; nextCursor: string | null }>;
  AgentLoad(id: string): Promise<AgentConversation>;
  AgentSnapshot(id: string): Promise<AgentConversation>;
  AgentSend(
    request: AgentSendRequest,
  ): Promise<{ conversation: AgentConversation; error: string }>;
  AgentStop(id: string): Promise<void>;
  AgentAnswer(answer: AgentAnswer): Promise<void>;
};

async function call<T>(
  native: keyof AgentBackend,
  route: string,
  body: unknown = {},
  args: unknown[] = [],
): Promise<T> {
  if (isDesktop()) {
    const app = window.go?.main?.App as unknown as AgentBackend;
    const method = app[native] as (...params: unknown[]) => Promise<T>;
    if (!method)
      throw new Error(
        'Restart the desktop app to load the updated Agent backend.',
      );
    return method(...args);
  }
  const response = await fetch('/__praewa_agent/' + route, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const text = await response.text();
  let data;
  try {
    data = JSON.parse(text);
  } catch {
    throw new Error(
      'Open the desktop app or the development preview to use your agent.',
    );
  }
  if (!response.ok) throw new Error(data.error || 'Agent request failed');
  return data as T;
}

export const agentBackend = {
  connect: () => call<AgentConnection>('AgentConnect', 'connect'),
  status: () => call<AgentConnection>('AgentStatus', 'status'),
  threads: (cursor = '') =>
    call<{ data: AgentThread[]; nextCursor: string | null }>(
      'AgentThreads',
      'threads',
      { cursor },
      [cursor],
    ),
  load: (id: string) =>
    call<AgentConversation>('AgentLoad', 'load', { id }, [id]),
  snapshot: (id: string) =>
    call<AgentConversation>('AgentSnapshot', 'snapshot', { id }, [id]),
  send: (send: AgentSendRequest) =>
    call<{ conversation: AgentConversation; error: string }>(
      'AgentSend',
      'send',
      { send },
      [send],
    ),
  stop: (id: string) => call<void>('AgentStop', 'stop', { id }, [id]),
  answer: (answer: AgentAnswer) =>
    call<void>('AgentAnswer', 'answer', { answer }, [answer]),
};

export function onAgentEvent(callback: (snapshot: AgentConversation) => void) {
  return onEvent<{ threadId: string; snapshot?: AgentConversation }>(
    'agent:event',
    (event) => {
      if (event.snapshot) callback(event.snapshot);
    },
  );
}
