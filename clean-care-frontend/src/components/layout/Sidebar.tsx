import { useEffect, useState } from 'react';
import { useNavigate, useLocation, useParams, Link } from 'react-router-dom';
import { listConversations } from '../../api/conversations';
import type { Conversation } from '../../types/conversation';
import { useAppStore } from '../../store/appStore';
import ThemeToggle from '../ui/ThemeToggle';

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

export default function Sidebar() {
  const { sidebarOpen, toggleSidebar } = useAppStore();
  const [conversations, setConversations] = useState<Conversation[]>(loadConversations);
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { conversationId } = useParams();

  // Sync conversations on route change
  useEffect(() => {
    setConversations(loadConversations());
  }, [conversationId]);

  // Listen for storage updates
  useEffect(() => {
    const sync = () => setConversations(loadConversations());
    window.addEventListener(CONVERSATIONS_UPDATED_EVENT, sync);
    window.addEventListener('storage', sync);
    return () => {
      window.removeEventListener(CONVERSATIONS_UPDATED_EVENT, sync);
      window.removeEventListener('storage', sync);
    };
  }, []);

  // Fetch from server on mount
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    listConversations()
      .then((items) => {
        if (cancelled) return;
        setConversations(items);
        saveConversations(items);
      })
      .catch(() => { /* use local data */ })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, []);

  const handleNew = () => {
    navigate('/chat');
    // On mobile, close sidebar after action
    if (window.innerWidth < 768) toggleSidebar();
  };

  const handleSelect = (conv: Conversation) => {
    navigate(`/chat/${conv.conversation_id}`);
    if (window.innerWidth < 768) toggleSidebar();
  };

  const isActive = (path: string) => {
    if (path === '/chat') return location.pathname.startsWith('/chat');
    if (path === '/admin') return location.pathname.startsWith('/admin');
    return false;
  };

  // Group conversations by date
  const grouped = groupConversations(conversations);

  return (
    <>
      {/* Mobile overlay */}
      {sidebarOpen && (
        <div className="sidebar-overlay" onClick={toggleSidebar} />
      )}
      <aside className={`sidebar ${sidebarOpen ? 'sidebar--open' : ''}`}>
        {/* Header: Logo + New Chat */}
        <div className="sidebar__header">
          <Link to="/chat" className="sidebar__logo">
            ♦ CleanCare
          </Link>
          <button
            className="sidebar__new-chat"
            onClick={handleNew}
            title="新建对话"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 5v14M5 12h14" />
            </svg>
          </button>
        </div>

        {/* Conversation list */}
        <div className="sidebar__conversations">
          {loading && conversations.length === 0 && (
            <div className="sidebar__loading">加载中...</div>
          )}
          {Object.entries(grouped).map(([label, items]) => (
            <div key={label} className="sidebar__group">
              <div className="sidebar__group-label">{label}</div>
              {items.map((conv) => (
                <button
                  key={conv.conversation_id}
                  className={`sidebar__item ${
                    conv.conversation_id === conversationId
                      ? 'sidebar__item--active'
                      : ''
                  }`}
                  onClick={() => handleSelect(conv)}
                  title={conv.title}
                >
                  <span className="sidebar__item-text">{conv.title}</span>
                </button>
              ))}
            </div>
          ))}
          {!loading && conversations.length === 0 && (
            <div className="sidebar__empty">还没有对话记录</div>
          )}
        </div>

        {/* Footer: Navigation + Theme */}
        <div className="sidebar__footer">
          <nav className="sidebar__nav">
            <Link
              to="/chat"
              className={`sidebar__nav-item ${isActive('/chat') ? 'sidebar__nav-item--active' : ''}`}
            >
              💬 对话
            </Link>
            <Link
              to="/admin"
              className={`sidebar__nav-item ${isActive('/admin') ? 'sidebar__nav-item--active' : ''}`}
            >
              ⚙️ 管理
            </Link>
          </nav>
          <div className="sidebar__theme">
            <ThemeToggle />
          </div>
        </div>
      </aside>
    </>
  );
}

function groupConversations(
  conversations: Conversation[]
): Record<string, Conversation[]> {
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getTime() - 86400000);
  const weekAgo = new Date(today.getTime() - 7 * 86400000);

  const groups: Record<string, Conversation[]> = {};

  for (const conv of conversations) {
    const date = new Date(conv.last_message_at || conv.created_at);
    let label: string;

    if (date >= today) {
      label = '今天';
    } else if (date >= yesterday) {
      label = '昨天';
    } else if (date >= weekAgo) {
      label = '最近7天';
    } else {
      label = '更早';
    }

    const group = groups[label] ?? (groups[label] = []);
    group.push(conv);
  }

  return groups;
}
