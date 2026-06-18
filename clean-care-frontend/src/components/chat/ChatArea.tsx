import { useEffect, useRef, useCallback, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import type { Message } from '../../types/conversation';
import LoadingSpinner from '../ui/LoadingSpinner';
import ErrorMessage from '../ui/ErrorMessage';
import { formatTime, copyToClipboard } from '../../utils/format';
import MessageActions from './MessageActions';

interface ChatAreaProps {
  messages: Message[];
  streamingContent: string;
  isLoading: boolean;
  error: string | null;
  onRetryLoad: () => void;
  onSuggestionClick?: (text: string) => void;
}

const SUGGESTIONS = [
  { label: '选购', text: '家庭地面，100平，预算5000，推荐扫地机器人' },
  { label: '对比', text: 'T20 和 X20 Pro 哪个更适合养宠物家庭？' },
  { label: '耗材', text: 'P400 空气净化器滤芯该换了吗？帮我查价格和库存' },
  { label: '排障', text: 'W300 净水器出水变小，怎么一步步排查？' },
];

const SUPPORTED_SCOPES = ['扫地机器人', '空气净化器', '净水器', '加湿器', '订单/售后'];

export default function ChatArea({
  messages,
  streamingContent,
  isLoading,
  error,
  onRetryLoad,
  onSuggestionClick,
}: ChatAreaProps) {
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const isNearBottomRef = useRef(true);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const handleEvidenceClick = useCallback((evidenceId: string) => {
    window.dispatchEvent(
      new CustomEvent('evidence-click', { detail: { evidenceId } })
    );
  }, []);

  const handleCopy = useCallback(async (messageId: string, content: string) => {
    const success = await copyToClipboard(content);
    if (success) {
      setCopiedId(messageId);
      setTimeout(() => setCopiedId(null), 2000);
    }
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
        <LoadingSpinner text="加载消息中..." />
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

  // Welcome screen with business-specific suggestions
  if (messages.length === 0 && !streamingContent) {
    return (
      <div className="chat-welcome">
        <div className="chat-welcome__hero">
          <div className="chat-welcome__icon">♦</div>
          <h1 className="chat-welcome__title">CleanCare 智能客服</h1>
          <p className="chat-welcome__subtitle">
            支持清洁电器选购、参数对比、耗材兼容、故障排查和订单售后查询
          </p>
          <div className="chat-welcome__scope" aria-label="支持范围">
            {SUPPORTED_SCOPES.map((scope) => (
              <span key={scope}>{scope}</span>
            ))}
          </div>
        </div>
        <div className="chat-welcome__suggestions">
          {SUGGESTIONS.map((s) => (
            <button
              key={s.text}
              className="chat-welcome__suggestion"
              onClick={() => onSuggestionClick?.(s.text)}
            >
              <span className="chat-welcome__suggestion-icon">{s.label}</span>
              <span>{s.text}</span>
            </button>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="chat-messages" ref={containerRef}>
      {messages.map((msg) => (
        <div key={msg.id} className={`message message--${msg.role}`}>
          <div className="message__avatar" aria-hidden="true">
            {msg.role === 'user' ? '👤' : '♦'}
          </div>
          <div className="message__content">
            <div className="message__body">
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
                            title={`查看证据 ${evidenceId}`}
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
                <span className="message__user-text">{msg.content}</span>
              )}
            </div>
            <div className="message__footer">
              <span className="message__time">
                {formatTime(msg.created_at || '')}
              </span>
              <div className="message__actions">
                <button
                  className={`message__copy ${copiedId === msg.id ? 'message__copy--copied' : ''}`}
                  onClick={() => handleCopy(msg.id, msg.content)}
                  title={copiedId === msg.id ? '已复制' : '复制'}
                >
                  {copiedId === msg.id ? '✓ 已复制' : '📋 复制'}
                </button>
                <MessageActions message={msg} />
              </div>
            </div>
          </div>
        </div>
      ))}

      {/* Streaming message */}
      {streamingContent && (
        <div className="message message--assistant">
          <div className="message__avatar" aria-hidden="true">♦</div>
          <div className="message__content">
            <div className="message__body">
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
                          title={`查看证据 ${evidenceId}`}
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
              <span className="streaming-indicator" />
            </div>
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
