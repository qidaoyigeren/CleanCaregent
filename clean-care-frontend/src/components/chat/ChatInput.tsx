import { useState, useRef, useCallback, useEffect } from 'react';
import type { KeyboardEvent } from 'react';

interface ChatInputProps {
  onSend: (content: string) => void;
  onAbort: () => void;
  isStreaming: boolean;
  disabled?: boolean;
}

export default function ChatInput({ onSend, onAbort, isStreaming, disabled }: ChatInputProps) {
  const [value, setValue] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Auto-resize textarea
  useEffect(() => {
    const el = textareaRef.current;
    if (el) {
      el.style.height = 'auto';
      el.style.height = Math.min(el.scrollHeight, 120) + 'px';
    }
  }, [value]);

  // Focus textarea on mount
  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const handleSend = useCallback(() => {
    const trimmed = value.trim();
    if (!trimmed || isStreaming) return;
    onSend(trimmed);
    setValue('');
  }, [value, isStreaming, onSend]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend]
  );

  return (
    <div className="chat-input">
      <textarea
        ref={textareaRef}
        className="chat-input__textarea"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={isStreaming ? 'Agent is responding...' : 'Ask a question... (Enter to send, Shift+Enter for new line)'}
        rows={1}
        disabled={isStreaming || disabled}
        readOnly={disabled && !isStreaming}
      />
      {isStreaming ? (
        <button
          className="chat-input__send"
          onClick={onAbort}
          title="Stop generating"
          style={{ background: 'var(--color-error)' }}
        >
          {'■'}
        </button>
      ) : (
        <button
          className="chat-input__send"
          onClick={handleSend}
          disabled={!value.trim()}
          title="Send message"
        >
          {'↑'}
        </button>
      )}
    </div>
  );
}
