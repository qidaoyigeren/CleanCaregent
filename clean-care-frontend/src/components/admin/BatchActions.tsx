import { useState } from 'react';

interface BatchActionsProps {
  selectedCount: number;
  onDelete?: () => void;
  onExport?: () => void;
  onClearSelection?: () => void;
}

export default function BatchActions({
  selectedCount,
  onDelete,
  onExport,
  onClearSelection,
}: BatchActionsProps) {
  const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);

  if (selectedCount === 0) {
    return null;
  }

  const handleDelete = () => {
    if (isConfirmingDelete) {
      onDelete?.();
      setIsConfirmingDelete(false);
    } else {
      setIsConfirmingDelete(true);
      // Auto-cancel after 3 seconds
      setTimeout(() => setIsConfirmingDelete(false), 3000);
    }
  };

  return (
    <div className="batch-actions">
      <div className="batch-actions__info">
        <span className="batch-actions__count">已选择 {selectedCount} 项</span>
        <button className="batch-actions__clear" onClick={onClearSelection}>
          取消选择
        </button>
      </div>
      <div className="batch-actions__buttons">
        {onExport && (
          <button className="batch-actions__btn batch-actions__btn--export" onClick={onExport}>
            📥 导出
          </button>
        )}
        {onDelete && (
          <button
            className={`batch-actions__btn batch-actions__btn--delete ${
              isConfirmingDelete ? 'batch-actions__btn--confirming' : ''
            }`}
            onClick={handleDelete}
          >
            {isConfirmingDelete ? '⚠️ 确认删除' : '🗑️ 删除'}
          </button>
        )}
      </div>
    </div>
  );
}
