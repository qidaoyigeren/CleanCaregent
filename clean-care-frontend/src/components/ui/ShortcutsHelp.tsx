import { useState, useEffect } from 'react';

interface Shortcut {
  keys: string[];
  description: string;
}

export default function ShortcutsHelp() {
  const [isOpen, setIsOpen] = useState(false);
  const [shortcuts, setShortcuts] = useState<Shortcut[]>([]);

  useEffect(() => {
    const handleShowHelp = (e: Event) => {
      const detail = (e as CustomEvent).detail;
      setShortcuts(detail.shortcuts || []);
      setIsOpen(true);
    };

    const handleEscape = () => {
      setIsOpen(false);
    };

    window.addEventListener('show-shortcuts-help', handleShowHelp);
    window.addEventListener('escape-pressed', handleEscape);

    return () => {
      window.removeEventListener('show-shortcuts-help', handleShowHelp);
      window.removeEventListener('escape-pressed', handleEscape);
    };
  }, []);

  if (!isOpen) return null;

  return (
    <div className="shortcuts-help-overlay" onClick={() => setIsOpen(false)}>
      <div className="shortcuts-help" onClick={(e) => e.stopPropagation()}>
        <div className="shortcuts-help__header">
          <h3>键盘快捷键</h3>
          <button className="shortcuts-help__close" onClick={() => setIsOpen(false)}>
            ✕
          </button>
        </div>
        <div className="shortcuts-help__content">
          {shortcuts.map((shortcut, index) => (
            <div key={index} className="shortcuts-help__item">
              <div className="shortcuts-help__keys">
                {shortcut.keys.map((key, i) => (
                  <span key={i}>
                    <kbd>{key}</kbd>
                    {i < shortcut.keys.length - 1 && <span> + </span>}
                  </span>
                ))}
              </div>
              <span className="shortcuts-help__description">{shortcut.description}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
