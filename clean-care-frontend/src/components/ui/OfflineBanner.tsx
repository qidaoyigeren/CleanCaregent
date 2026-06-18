import { useEffect, useState } from 'react';
import useOffline from '../../hooks/useOffline';

/**
 * Banner that appears when the app is offline.
 * Also shows a "reconnected" message briefly when coming back online.
 */
export default function OfflineBanner() {
  const { isOffline, wasOffline, clearWasOffline } = useOffline();
  const [showReconnected, setShowReconnected] = useState(false);

  // When coming back online, show "reconnected" briefly
  useEffect(() => {
    if (wasOffline && !isOffline) {
      setShowReconnected(true);
      const timer = setTimeout(() => {
        setShowReconnected(false);
        clearWasOffline();
      }, 3000);
      return () => clearTimeout(timer);
    }
  }, [wasOffline, isOffline, clearWasOffline]);

  if (!isOffline && !showReconnected) return null;

  return (
    <div
      className={`offline-banner ${isOffline ? 'offline-banner--offline' : 'offline-banner--reconnected'}`}
      role="status"
      aria-live="polite"
    >
      {isOffline ? (
        <>
          <span className="offline-banner__icon">📡</span>
          <span>网络已断开 — 查看历史记录可用，发送消息需等待恢复</span>
        </>
      ) : (
        <>
          <span className="offline-banner__icon">✅</span>
          <span>网络已恢复</span>
        </>
      )}
    </div>
  );
}
