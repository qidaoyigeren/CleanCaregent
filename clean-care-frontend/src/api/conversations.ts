import { apiGet, apiPost } from './client';
import type { Conversation, Message } from '../types/conversation';
import type { PaginatedItems } from '../types/api';

export function createConversation(title: string): Promise<Conversation> {
  return apiPost<Conversation>('/conversations', { title });
}

export function getMessages(
  conversationId: string,
  limit: number = 20,
  cursor?: string
): Promise<PaginatedItems<Message>> {
  let path = `/conversations/${conversationId}/messages?limit=${limit}`;
  if (cursor) path += `&cursor=${cursor}`;
  return apiGet<PaginatedItems<Message>>(path);
}

export function sendMessage(
  conversationId: string,
  content: string
): Promise<{ message_id: string; answer: string; evidences: unknown[]; trace_id: string; mode: string }> {
  return apiPost(`/conversations/${conversationId}/messages`, { content });
}
