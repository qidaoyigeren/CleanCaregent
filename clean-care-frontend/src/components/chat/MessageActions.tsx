import { useState, useRef, useEffect } from 'react';
import type { Message } from '../../types/conversation';
import { copyToClipboard } from '../../utils/format';

interface MessageActionsProps {
  message: Message;
}

export default function MessageActions({ message }: MessageActionsProps) {
  const [isOpen, setIsOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const handleCopy = async () => {
    await copyToClipboard(message.content);
    setIsOpen(false);
  };

  const handleCopyWithFormat = async () => {
    const formatted = `[${message.role}]\n${message.content}`;
    await copyToClipboard(formatted);
    setIsOpen(false);
  };

  return (
    <div className="message-actions" ref={menuRef}>
      <button
        className="message-actions__trigger"
        onClick={() => setIsOpen(!isOpen)}
        title="更多操作"
      >
        ⋯
      </button>
      {isOpen && (
        <div className="message-actions__menu">
          <button onClick={handleCopy}>
            <span className="message-actions__icon">📋</span>
            复制内容
          </button>
          <button onClick={handleCopyWithFormat}>
            <span className="message-actions__icon">📝</span>
            复制（带格式）
          </button>
          {message.role === 'assistant' && message.trace_id && (
            <button onClick={() => window.open(`/traces/${message.trace_id}`, '_blank')}>
              <span className="message-actions__icon">🔍</span>
              查看Trace
            </button>
          )}
        </div>
      )}
    </div>
  );
}
