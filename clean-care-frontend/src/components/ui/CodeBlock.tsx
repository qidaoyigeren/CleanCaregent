import { useEffect, useRef, useState } from 'react';
import hljs from 'highlight.js';
import { copyToClipboard } from '../../utils/format';
import { syncHljsTheme } from '../../utils/hljsTheme';

interface CodeBlockProps {
  language?: string;
  children: string;
}

export default function CodeBlock({ language, children }: CodeBlockProps) {
  const codeRef = useRef<HTMLElement>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    // Ensure the correct theme CSS is loaded before highlighting
    syncHljsTheme();
    if (codeRef.current) {
      hljs.highlightElement(codeRef.current);
    }
  }, [children, language]);

  const handleCopy = async () => {
    const success = await copyToClipboard(children);
    if (success) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <div className="code-block">
      <div className="code-block__header">
        <span className="code-block__language">{language || 'code'}</span>
        <button
          className={`code-block__copy ${copied ? 'code-block__copy--copied' : ''}`}
          onClick={handleCopy}
        >
          {copied ? '✓ 已复制' : '复制'}
        </button>
      </div>
      <pre className="code-block__pre">
        <code ref={codeRef} className={`hljs language-${language || 'plaintext'}`}>
          {children}
        </code>
      </pre>
    </div>
  );
}
