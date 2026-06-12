import { apiPost, apiFetch } from './client';

interface SearchResult {
  chunk_id: string;
  document_id: string;
  title: string;
  content: string;
  score: number;
  doc_type?: string;
  metadata?: Record<string, unknown>;
}

interface SearchResponse {
  results: SearchResult[];
  total: number;
}

export function searchKnowledge(
  query: string,
  mode: 'hybrid' | 'dense' | 'keyword' = 'hybrid',
  filters?: { models?: string[] }
): Promise<SearchResponse> {
  return apiPost<SearchResponse>('/admin/kb/search', { query, mode, filter: filters });
}

export function uploadDocument(formData: FormData): Promise<{ document_id: string }> {
  return apiFetch<{ document_id: string }>('/admin/kb/documents', {
    method: 'POST',
    body: formData,
  });
}

export type { SearchResult, SearchResponse };
