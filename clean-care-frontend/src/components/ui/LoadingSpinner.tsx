export default function LoadingSpinner({ text }: { text?: string }) {
  return (
    <div className="loading-spinner">
      <div className="loading-spinner__ring" />
      {text && <span style={{ marginLeft: 12, fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)' }}>{text}</span>}
    </div>
  );
}
