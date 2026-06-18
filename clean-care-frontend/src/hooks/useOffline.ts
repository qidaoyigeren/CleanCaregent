import { useState, useEffect, useCallback } from 'react';

interface OfflineState {
  isOffline: boolean;
  wasOffline: boolean; // True if we just came back online
  clearWasOffline: () => void;
}

/**
 * Hook to detect online/offline status.
 * Tracks both current state and "just reconnected" state.
 */
export default function useOffline(): OfflineState {
  const [isOffline, setIsOffline] = useState(!navigator.onLine);
  const [wasOffline, setWasOffline] = useState(false);

  const handleOffline = useCallback(() => {
    setIsOffline(true);
  }, []);

  const handleOnline = useCallback(() => {
    setIsOffline(false);
    setWasOffline(true);
  }, []);

  const clearWasOffline = useCallback(() => {
    setWasOffline(false);
  }, []);

  useEffect(() => {
    window.addEventListener('offline', handleOffline);
    window.addEventListener('online', handleOnline);

    return () => {
      window.removeEventListener('offline', handleOffline);
      window.removeEventListener('online', handleOnline);
    };
  }, [handleOffline, handleOnline]);

  return { isOffline, wasOffline, clearWasOffline };
}
