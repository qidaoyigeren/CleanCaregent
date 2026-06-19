import { useState, useRef, useCallback, useEffect } from 'react';
import type { KeyboardEvent } from 'react';

interface ChatInputProps {
  onSend: (content: string) => void;
  onAbort: () => void;
  onHandoff?: () => void;
  isStreaming: boolean;
  disabled?: boolean;
}

export default function ChatInput({
  onSend,
  onAbort,
  onHandoff,
  isStreaming,
  disabled,
}: ChatInputProps) {
  const [value, setValue] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    const el = textareaRef.current;
    if (el) {
      el.style.height = 'auto';
      el.style.height = Math.min(el.scrollHeight, 200) + 'px';
    }
  }, [value]);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const handleSend = useCallback(() => {
    const trimmed = value.trim();
    if (!trimmed || isStreaming) return;
    onSend(trimmed);
    setValue('');
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
    }
  }, [value, isStreaming, onSend]);

  const handleHandoff = useCallback(() => {
    if (isStreaming || disabled) return;
    if (onHandoff) {
      onHandoff();
      return;
    }
    onSend('请转人工客服接管当前问题');
  }, [disabled, isStreaming, onHandoff, onSend]);

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
      <div className="chat-input__wrapper">
        <textarea
          ref={textareaRef}
          className="chat-input__textarea"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={isStreaming ? '正在回复中...' : '输入选购、故障、订单、保修或售后问题...'}
          rows={1}
          disabled={isStreaming || disabled}
          readOnly={disabled && !isStreaming}
        />
        <div className="chat-input__buttons">
          {isStreaming ? (
            <button
              className="chat-input__stop"
              onClick={onAbort}
              title="停止生成"
              aria-label="停止生成"
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <rect x="6" y="6" width="12" height="12" rx="2" />
              </svg>
            </button>
          ) : (
            <>
              <button
                className="chat-input__handoff"
                onClick={handleHandoff}
                disabled={disabled}
                title="转人工客服"
                aria-label="转人工客服"
              >
                人工
              </button>
              <button
                className="chat-input__send"
                onClick={handleSend}
                disabled={!value.trim() || disabled}
                title="发送消息"
                aria-label="发送消息"
              >
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <path d="M12 19V5M5 12l7-7 7 7" />
                </svg>
              </button>
            </>
          )}
        </div>
      </div>
      <div className="chat-input__hint">
        Enter 发送，Shift+Enter 换行。需要人工处理时可直接点“人工”。
      </div>
    </div>
  );
}
