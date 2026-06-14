import { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { listConversations } from '../../api/conversations';
import type { Conversation } from '../../types/conversation';

const STORAGE_KEY = 'cleancare_conversations';
const CONVERSATIONS_UPDATED_EVENT = 'cleancare-conversations-updated';

function loadConversations(): Conversation[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function saveConversations(list: Conversation[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
}

export function addConversationToStorage(conv: Conversation) {
  const list = loadConversations();
  const index = list.findIndex(
    (item) => item.conversation_id === conv.conversation_id
  );
  if (index >= 0) {
    list[index] = conv;
  } else {
    list.unshift(conv);
  }
  saveConversations(list);
  window.dispatchEvent(new Event(CONVERSATIONS_UPDATED_EVENT));
}

interface ConversationListProps {
  onSelect?: () => void;
}

export default function ConversationList({ onSelect }: ConversationListProps) {
  const [open, setOpen] = useState(false);
  const [conversations, setConversations] =
    useState<Conversation[]>(loadConversations);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const navigate = useNavigate();
  const { conversationId } = useParams();
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setConversations(loadConversations());
  }, [conversationId]);

  useEffect(() => {
    const syncLocal = () => setConversations(loadConversations());
    window.addEventListener(CONVERSATIONS_UPDATED_EVENT, syncLocal);
    window.addEventListener('storage', syncLocal);
    return () => {
      window.removeEventListener(CONVERSATIONS_UPDATED_EVENT, syncLocal);
      window.removeEventListener('storage', syncLocal);
    };
  }, []);

  useEffect(() => {
    if (!open) return;

    let cancelled = false;
    setLoading(true);
    setLoadError(false);
    listConversations()
      .then((items) => {
        if (cancelled) return;
        setConversations(items);
        saveConversations(items);
      })
      .catch(() => {
        if (!cancelled) setLoadError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [open]);

  useEffect(() => {
    function handleClick(event: MouseEvent) {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        setOpen(false);
      }
    }
    if (open) {
      document.addEventListener('mousedown', handleClick);
      return () => document.removeEventListener('mousedown', handleClick);
    }
  }, [open]);

  const handleNew = () => {
    navigate('/chat');
    setOpen(false);
    onSelect?.();
  };

  const handleSelect = (conversation: Conversation) => {
    navigate(`/chat/${conversation.conversation_id}`);
    setOpen(false);
    onSelect?.();
  };

  const activeTitle = conversations.find(
    (conversation) => conversation.conversation_id === conversationId
  )?.title;

  return (
    <div className="conv-list" ref={ref}>
      <button
        className="conv-list__toggle"
        onClick={() => setOpen((value) => !value)}
        title={activeTitle || '历史对话'}
      >
        <span aria-hidden="true">&#x2630;</span>
        <span>{activeTitle || '历史对话'}</span>
        <span aria-hidden="true">{open ? '\u25B2' : '\u25BC'}</span>
      </button>

      {open && (
        <div className="conv-list__dropdown">
          <button className="conv-list__new-btn" onClick={handleNew}>
            + 新建对话
          </button>
          <div className="conv-list__heading">
            <span>最近对话</span>
            {loading && <span className="conv-list__loading">刷新中...</span>}
          </div>
          {loadError && (
            <div className="conv-list__notice">
              服务端历史加载失败，已显示本地记录
            </div>
          )}
          {!loading && conversations.length === 0 ? (
            <div className="conv-list__empty">还没有历史对话</div>
          ) : (
            conversations.map((conversation) => (
              <button
                key={conversation.conversation_id}
                className={`conv-list__item ${
                  conversation.conversation_id === conversationId
                    ? 'conv-list__item--active'
                    : ''
                }`}
                onClick={() => handleSelect(conversation)}
              >
                <span className="conv-list__item-title">{conversation.title}</span>
                <span className="conv-list__item-time">
                  {formatConversationTime(
                    conversation.last_message_at || conversation.created_at
                  )}
                </span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}

function formatConversationTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return new Intl.DateTimeFormat('zh-CN', {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

export { loadConversations, saveConversations };
