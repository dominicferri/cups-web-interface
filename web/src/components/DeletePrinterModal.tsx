import React, { useState, useId } from 'react';
import { Trash2, AlertTriangle, Loader2 } from 'lucide-react';
import type { Printer } from '../types/cups';
import { useModalDialog } from '../utils/useModalDialog';

interface DeletePrinterModalProps {
  printer: Printer | null;
  isOpen: boolean;
  onClose: () => void;
  onConfirmDelete: (name: string) => Promise<void>;
}

export const DeletePrinterModal: React.FC<DeletePrinterModalProps> = ({
  printer,
  isOpen,
  onClose,
  onConfirmDelete,
}) => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const titleId = useId();
  const dialogRef = useModalDialog(Boolean(isOpen && printer), onClose);

  if (!isOpen || !printer) return null;

  const handleDelete = async () => {
    setLoading(true);
    setError(null);
    try {
      await onConfirmDelete(printer.name);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete printer queue');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/50 backdrop-blur-xs animate-in fade-in duration-150">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-2xl shadow-2xl max-w-md w-full p-6 space-y-5 animate-in zoom-in-95 duration-150 focus:outline-none"
      >
        <div className="flex items-start space-x-4">
          <div className="w-11 h-11 rounded-2xl bg-rose-50 dark:bg-rose-950/60 border border-rose-200 dark:border-rose-900/60 text-rose-600 dark:text-rose-400 flex items-center justify-center shrink-0">
            <Trash2 className="w-5 h-5" aria-hidden="true" />
          </div>
          <div className="flex-1 min-w-0">
            <h3 id={titleId} className="text-base font-bold text-slate-900 dark:text-white">
              Delete Printer Queue
            </h3>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 leading-relaxed">
              Are you sure you want to delete <span className="font-semibold text-slate-900 dark:text-slate-200">"{printer.name}"</span>?
            </p>
          </div>
        </div>

        <div className="p-3.5 rounded-xl bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-900/60 text-amber-900 dark:text-amber-200 text-xs flex items-start space-x-2.5">
          <AlertTriangle className="w-4 h-4 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" aria-hidden="true" />
          <div>
            This will permanently remove the queue via <code className="font-mono bg-amber-100 dark:bg-amber-900/60 px-1 py-0.5 rounded text-xs">lpadmin -x {printer.name}</code>. Any active or pending print jobs in this queue will be cancelled.
          </div>
        </div>

        {error && (
          <div
            role="alert"
            className="p-3 rounded-xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-900 text-rose-700 dark:text-rose-300 text-xs"
          >
            {error}
          </div>
        )}

        <div className="flex items-center justify-end space-x-3 pt-2">
          <button
            type="button"
            onClick={onClose}
            disabled={loading}
            className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleDelete}
            disabled={loading}
            className="inline-flex items-center space-x-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-rose-600 hover:bg-rose-700 disabled:opacity-50 transition shadow-xs cursor-pointer"
          >
            {loading ? (
              <>
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
                <span>Deleting...</span>
              </>
            ) : (
              <>
                <Trash2 className="w-3.5 h-3.5" />
                <span>Delete Queue</span>
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};
