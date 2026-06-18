import { useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';

interface ShortcutHandlers {
  onNewConversation?: () => void;
  onToggleSidebar?: () => void;
  onTogglePipeline?: () => void;
  onSearch?: () => void;
}

export function useKeyboardShortcuts(handlers: ShortcutHandlers = {}) {
  const navigate = useNavigate();

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      const isModifier = e.ctrlKey || e.metaKey;

      // Ctrl/Cmd + N: New conversation
      if (isModifier && e.key === 'n') {
        e.preventDefault();
        if (!handlers.onNewConversation) { navigate('/chat'); } else { handlers.onNewConversation(); }
      }

      // Ctrl/Cmd + B: Toggle sidebar
      if (isModifier && e.key === 'b') {
        e.preventDefault();
        handlers.onToggleSidebar?.();
      }

      // Ctrl/Cmd + J: Toggle pipeline panel
      if (isModifier && e.key === 'j') {
        e.preventDefault();
        handlers.onTogglePipeline?.();
      }

      // Ctrl/Cmd + K: Search
      if (isModifier && e.key === 'k') {
        e.preventDefault();
        handlers.onSearch?.();
      }

      // Ctrl/Cmd + /: Show shortcuts help
      if (isModifier && e.key === '/') {
        e.preventDefault();
        showShortcutsHelp();
      }

      // Escape: Close modals/menus
      if (e.key === 'Escape') {
        document.dispatchEvent(new CustomEvent('escape-pressed'));
      }
    },
    [handlers, navigate]
  );

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);
}

function showShortcutsHelp() {
  document.dispatchEvent(
    new CustomEvent('show-shortcuts-help', {
      detail: {
        shortcuts: [
          { keys: ['Ctrl', 'N'], description: '新建对话' },
          { keys: ['Ctrl', 'B'], description: '切换侧边栏' },
          { keys: ['Ctrl', 'J'], description: '切换Pipeline面板' },
          { keys: ['Ctrl', 'K'], description: '搜索' },
          { keys: ['Ctrl', '/'], description: '显示快捷键帮助' },
          { keys: ['Escape'], description: '关闭弹窗' },
        ],
      },
    })
  );
}

export default useKeyboardShortcuts;
