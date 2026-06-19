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
  { label: '订单', text: '帮我查订单 CC20250522008 的状态和商品明细' },
  { label: '保修', text: '订单 CC20250522008 还在保修期吗？' },
  { label: '排障', text: 'W300 净水器出水变小，怎么一步步排查？' },
  { label: '售后', text: '订单 CC20250522008 的维修进度到哪了？' },
];

const SUPPORTED_SCOPES = ['扫地机器人', '空气净化器', '净水器', '加湿器', '订单售后'];

type ServiceCard = {
  kind: 'order' | 'warranty' | 'fault' | 'handoff';
  title: string;
  detail: string;
};

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

  const checkNearBottom = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  }, []);

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

  if (messages.length === 0 && !streamingContent) {
    return (
      <div className="chat-welcome">
        <div className="chat-welcome__hero">
          <div className="chat-welcome__icon">CC</div>
          <h1 className="chat-welcome__title">CleanCare 智能客服</h1>
          <p className="chat-welcome__subtitle">
            支持清洁电器选购、参数对比、耗材兼容、故障排查、订单保修、退换退款和人工接管。
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
            {msg.role === 'user' ? '你' : 'AI'}
          </div>
          <div className="message__content">
            <ServiceCards content={msg.role === 'assistant' ? msg.content : ''} />
            <div className="message__body">
              {msg.role === 'assistant' ? (
                <MarkdownBody content={msg.content} onEvidenceClick={handleEvidenceClick} />
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
                  {copiedId === msg.id ? '已复制' : '复制'}
                </button>
                <MessageActions message={msg} />
              </div>
            </div>
          </div>
        </div>
      ))}

      {streamingContent && (
        <div className="message message--assistant">
          <div className="message__avatar" aria-hidden="true">AI</div>
          <div className="message__content">
            <ServiceCards content={streamingContent} />
            <div className="message__body">
              <MarkdownBody content={streamingContent} onEvidenceClick={handleEvidenceClick} />
              <span className="streaming-indicator" />
            </div>
          </div>
        </div>
      )}

      <div ref={messagesEndRef} />
    </div>
  );
}

function MarkdownBody({
  content,
  onEvidenceClick,
}: {
  content: string;
  onEvidenceClick: (evidenceId: string) => void;
}) {
  return (
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
                  onEvidenceClick(evidenceId);
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
      {linkifyEvidenceTags(content)}
    </ReactMarkdown>
  );
}

function ServiceCards({ content }: { content: string }) {
  const cards = serviceCardsForContent(content);
  if (cards.length === 0) return null;
  return (
    <div className="service-cards" aria-label="售后状态摘要">
      {cards.map((card) => (
        <div key={card.kind} className={`service-card service-card--${card.kind}`}>
          <div className="service-card__label">{card.title}</div>
          <div className="service-card__detail">{card.detail}</div>
        </div>
      ))}
    </div>
  );
}

function serviceCardsForContent(content: string): ServiceCard[] {
  const text = content || '';
  const cards: ServiceCard[] = [];
  const orderNo = text.match(/\b(?:CC|ORDER)[0-9]{6,}\b/i)?.[0]?.toUpperCase();
  const ticketNo = text.match(/\b(?:TICKET|AS|HANDOFF)[A-Z0-9_-]{6,}\b/i)?.[0]?.toUpperCase();

  if (orderNo || text.includes('订单状态') || text.includes('订单事实')) {
    cards.push({
      kind: 'order',
      title: '订单',
      detail: orderNo ? `订单号 ${orderNo}` : '已引用当前账号订单信息',
    });
  }
  if (text.includes('保修') || text.includes('在保')) {
    cards.push({
      kind: 'warranty',
      title: '保修',
      detail: text.includes('不在保') || text.includes('过保') ? '需要结合政策或人工复核' : '已进行保修状态判断',
    });
  }
  if (text.includes('故障') || text.includes('排查') || text.includes('维修进度') || text.includes('售后进度')) {
    cards.push({
      kind: 'fault',
      title: '故障/维修',
      detail: ticketNo ? `工单 ${ticketNo}` : '已进入排查或售后进度流程',
    });
  }
  if (text.includes('人工接管') || text.includes('转人工') || text.includes('人工客服') || text.includes('human_queued')) {
    cards.push({
      kind: 'handoff',
      title: '人工接管',
      detail: ticketNo ? `队列工单 ${ticketNo}` : '当前对话可排入人工队列',
    });
  }
  return cards;
}

function linkifyEvidenceTags(content: string): string {
  return content.replace(/\[(E\d+)\]/g, '[$1](#evidence-$1)');
}
