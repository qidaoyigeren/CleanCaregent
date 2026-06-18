import type { ConnectionState } from '../../hooks/useSSEStream';

interface ConnectionStatusProps {
  state: ConnectionState;
  reconnectCount: number;
}

export default function ConnectionStatus({ state, reconnectCount }: ConnectionStatusProps) {
  const getStatusInfo = () => {
    switch (state) {
      case 'idle':
        return null; // Don't show when idle
      case 'connecting':
        return { icon: '🔄', text: '连接中...', className: 'connecting' };
      case 'connected':
        return { icon: '🟢', text: '已连接', className: 'connected' };
      case 'reconnecting':
        return {
          icon: '🔄',
          text: `重连中 (${reconnectCount}/3)...`,
          className: 'reconnecting',
        };
      case 'disconnected':
        return { icon: '⚪', text: '已断开', className: 'disconnected' };
      case 'error':
        return { icon: '🔴', text: '连接错误', className: 'error' };
      default:
        return null;
    }
  };

  const info = getStatusInfo();
  if (!info) return null;

  return (
    <div className={`connection-status connection-status--${info.className}`}>
      <span className="connection-status__icon">{info.icon}</span>
      <span className="connection-status__text">{info.text}</span>
    </div>
  );
}
