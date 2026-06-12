import { useEffect, useRef, useCallback, type ReactNode } from 'react';
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

/** Split text on evidence tags [EX] and make them clickable */
function renderEvidenceContent(text: string, onEvidenceClick: (id: string) => void): ReactNode[] {
  const parts = text.split(/(\[E\d+\])/g);
  return parts.map((part, i) => {
    const match = part.match(/^\[(E\d+)\]$/);
    if (match) {
      const evidenceId = match[1]!;
      return (
        <span
          key={i}
          className="evidence-tag"
          onClick={(e) => {
            e.stopPropagation();
            onEvidenceClick(evidenceId);
          }}
          title={`View evidence ${evidenceId}`}
        >
          [{evidenceId}]
        </span>
      );
    }
    return <span key={i}>{part}</span>;
  });
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
                  // Override text rendering to make evidence tags clickable
                  p: ({ children }) => {
                    const text = extractTextFromChildren(children);
                    if (text && /\[E\d+\]/.test(text)) {
                      return <p>{renderEvidenceContent(text, handleEvidenceClick)}</p>;
                    }
                    return <p>{children}</p>;
                  },
                  li: ({ children }) => {
                    const text = extractTextFromChildren(children);
                    if (text && /\[E\d+\]/.test(text)) {
                      return <li>{renderEvidenceContent(text, handleEvidenceClick)}</li>;
                    }
                    return <li>{children}</li>;
                  },
                }}
              >
                {msg.content}
              </ReactMarkdown>
            ) : (
              msg.content.split('\n').map((line, i) => (
                <span key={i}>
                  {i > 0 && <br />}
                  {renderEvidenceContent(line, handleEvidenceClick)}
                </span>
              ))
            )}
          </div>
        </div>
      ))}

      {/* Streaming message */}
      {streamingContent && (
        <div className="message message--assistant">
          <div className="message__avatar" aria-hidden="true">🤖</div>
          <div className="message__bubble">
            <ReactMarkdown>{streamingContent}</ReactMarkdown>
            <span className="status-bar__dot" style={{ display: 'inline-block', marginLeft: 4 }} />
          </div>
        </div>
      )}

      <div ref={messagesEndRef} />
    </div>
  );
}

/** Extract plain text from React children (used for evidence tag detection) */
function extractTextFromChildren(children: ReactNode): string {
  if (typeof children === 'string') return children;
  if (Array.isArray(children)) {
    return children.map(c => (typeof c === 'string' ? c : '')).join('');
  }
  return '';
}
