import { useState, useEffect, useRef } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import type { Conversation } from '../../types/conversation';

const STORAGE_KEY = 'cleancare_conversations';

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
  const idx = list.findIndex((c) => c.conversation_id === conv.conversation_id);
  if (idx >= 0) {
    list[idx] = conv;
  } else {
    list.unshift(conv);
  }
  saveConversations(list);
}

interface ConversationListProps {
  onSelect?: () => void;
}

export default function ConversationList({ onSelect }: ConversationListProps) {
  const [open, setOpen] = useState(false);
  const [conversations, setConversations] = useState<Conversation[]>(loadConversations);
  const navigate = useNavigate();
  const { conversationId } = useParams();
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setConversations(loadConversations());
  }, [conversationId]);

  // Close dropdown on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
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

  const handleSelect = (conv: Conversation) => {
    navigate(`/chat/${conv.conversation_id}`);
    setOpen(false);
    onSelect?.();
  };

  const activeTitle = conversations.find((c) => c.conversation_id === conversationId)?.title;

  return (
    <div className="conv-list" ref={ref}>
      <button className="conv-list__toggle" onClick={() => setOpen(!open)} title={activeTitle}>
        <span>{'☰'}</span>
        <span>{activeTitle || 'Sessions'}</span>
        <span>{open ? '▲' : '▼'}</span>
      </button>

      {open && (
        <div className="conv-list__dropdown">
          <button className="conv-list__new-btn" onClick={handleNew}>
            {'+ New Session'}
          </button>
          {conversations.length === 0 ? (
            <div className="conv-list__empty">No conversations yet</div>
          ) : (
            conversations.map((conv) => (
              <button
                key={conv.conversation_id}
                className={`conv-list__item ${conv.conversation_id === conversationId ? 'conv-list__item--active' : ''}`}
                onClick={() => handleSelect(conv)}
              >
                {conv.title}
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}

export { loadConversations, saveConversations };
