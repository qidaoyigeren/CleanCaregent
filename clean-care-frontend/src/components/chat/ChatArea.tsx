import { useEffect, useRef, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import type { Message } from '../../types/conversation';
import LoadingSpinner from '../ui/LoadingSpinner';
import EmptyState from '../ui/EmptyState';
import ErrorMessage from '../ui/ErrorMessage';

interface ChatAreaProps {
  messages: Message[];
  streamingContent: string;
  isLoading: boolean;
  error: string | null;
  onRetryLoad: () => void;
}

export default function ChatArea({ messages, streamingContent, isLoading, error, onRetryLoad }: ChatAreaProps) {
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const isNearBottomRef = useRef(true);

  const handleEvidenceClick = useCallback((evidenceId: string) => {
    window.dispatchEvent(
      new CustomEvent('evidence-click', { detail: { evidenceId } })
    );
  }, []);

  // Track whether user is near bottom for smart auto-scroll
  const checkNearBottom = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  }, []);

  // Auto-scroll only when user is near the bottom
  useEffect(() => {
    if (isNearBottomRef.current) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, streamingContent]);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    el.addEventListener('scroll', checkNearBottom, { passive: true });
    return () => el.removeEventListener('scroll', checkNearBottom);
  }, [checkNearBottom]);

  if (isLoading) {
    return (
      <div className="chat-messages">
        <LoadingSpinner text="Loading messages..." />
      </div>
    );
  }

  if (error) {
    return (
      <div className="chat-messages">
        <div style={{ padding: 24 }}>
          <ErrorMessage message={error} onRetry={onRetryLoad} />
        </div>
      </div>
    );
  }

  if (messages.length === 0 && !streamingContent) {
    return (
      <div className="chat-messages">
        <EmptyState
          icon="💬"
          message="Start a conversation — ask about product specifications, prices, or troubleshooting."
        />
      </div>
    );
  }

  return (
    <div className="chat-messages" ref={containerRef}>
      {messages.map((msg) => (
        <div key={msg.id} className={`message message--${msg.role}`}>
          <div className="message__avatar" aria-hidden="true">
            {msg.role === 'user' ? '👤' : '🤖'}
          </div>
          <div className="message__bubble">
            {msg.role === 'assistant' ? (
              <ReactMarkdown
                components={{
                  a: ({ href, children }) => {
                    const match = href?.match(/^#evidence-(E\d+)$/);
                    if (match) {
                      const evidenceId = match[1]!;
                      return (
                        <span
                          className="evidence-tag"
                          onClick={(event) => {
                            event.stopPropagation();
                            handleEvidenceClick(evidenceId);
                          }}
                          title={`View evidence ${evidenceId}`}
                        >
                          [{evidenceId}]
                        </span>
                      );
                    }
                    return <a href={href}>{children}</a>;
                  },
                }}
              >
                {linkifyEvidenceTags(msg.content)}
              </ReactMarkdown>
            ) : (
              <span style={{ whiteSpace: 'pre-wrap' }}>{msg.content}</span>
            )}
          </div>
        </div>
      ))}

      {/* Streaming message */}
      {streamingContent && (
        <div className="message message--assistant">
          <div className="message__avatar" aria-hidden="true">🤖</div>
          <div className="message__bubble">
            <ReactMarkdown
              components={{
                a: ({ href, children }) => {
                  const match = href?.match(/^#evidence-(E\d+)$/);
                  if (match) {
                    const evidenceId = match[1]!;
                    return (
                      <span
                        className="evidence-tag"
                        onClick={() => handleEvidenceClick(evidenceId)}
                        title={`View evidence ${evidenceId}`}
                      >
                        [{evidenceId}]
                      </span>
                    );
                  }
                  return <a href={href}>{children}</a>;
                },
              }}
            >
              {linkifyEvidenceTags(streamingContent)}
            </ReactMarkdown>
            <span className="status-bar__dot" style={{ display: 'inline-block', marginLeft: 4 }} />
          </div>
        </div>
      )}

      <div ref={messagesEndRef} />
    </div>
  );
}

function linkifyEvidenceTags(content: string): string {
  return content.replace(/\[(E\d+)\]/g, '[$1](#evidence-$1)');
}
