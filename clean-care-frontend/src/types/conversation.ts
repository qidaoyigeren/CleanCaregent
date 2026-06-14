export interface Conversation {
  conversation_id: string;
  title: string;
  status: string;
  created_at: string;
  last_message_at: string;
}

export interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  trace_id?: string;
  created_at?: string;
}
