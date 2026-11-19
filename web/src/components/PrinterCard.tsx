import React, { useState } from 'react';
import {
  Printer as PrinterIcon,
  Star,
  Pause,
  Play,
  HardDrive,
  MapPin,
  AlertCircle,
  FileText,
  Trash2,
  Cpu,
  AlertTriangle,
  Sliders,
} from 'lucide-react';
import type { Printer } from '../types/cups';
import { formatAlert } from '../utils/format';

interface PrinterCardProps {
  printer: Printer;
  onPause: (name: string) => Promise<void>;
  onResume: (name: string) => Promise<void>;
  onSetDefault: (name: string) => Promise<void>;
  onPurge: (name: string) => Promise<void>;
  onConfigure?: (printer: Printer, tab?: 'preferences' | 'accessories' | 'audit') => void;
}

export const PrinterCard: React.FC<PrinterCardProps> = ({
  printer,
  onPause,
  onResume,
  onSetDefault,
  onPurge,
  onConfigure,
}) => {
  const [actionLoading, setActionLoading] = useState(false);
  const [confirmPurge, setConfirmPurge] = useState(false);

  const handleToggleState = async () => {
    setActionLoading(true);
    try {
      if (printer.state === 'stopped') {
        await onResume(printer.name);
      } else {
        await onPause(printer.name);
      }
    } finally {
      setActionLoading(false);
    }
  };

  const handleSetDefault = async () => {
    setActionLoading(true);
    try {
      await onSetDefault(printer.name);
    } finally {
      setActionLoading(false);
    }
  };

  const handlePurge = async () => {
    setActionLoading(true);
    try {
      await onPurge(printer.name);
      setConfirmPurge(false);
    } finally {
      setActionLoading(false);
    }
  };

  // State badge style calculation
  const getStateBadge = () => {
    switch (printer.state) {
      case 'idle':
        return {
          pill: 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-400 dark:border-emerald-800',
          dot: 'bg-emerald-500',
          label: 'Ready',
        };
      case 'printing':
        return {
          pill: 'bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-950/40 dark:text-blue-400 dark:border-blue-800',
          dot: 'bg-blue-500 animate-pulse',
          label: 'Printing',
        };
      case 'stopped':
        return {
          pill: 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-950/40 dark:text-amber-400 dark:border-amber-800',
          dot: 'bg-amber-500',
          label: 'Paused',
        };
      case 'error':
      default:
        return {
          pill: 'bg-rose-50 text-rose-700 border-rose-200 dark:bg-rose-950/40 dark:text-rose-400 dark:border-rose-800',
          dot: 'bg-rose-500',
          label: printer.state === 'error' ? 'Error' : printer.state,
        };
    }
  };

  const stateStyle = getStateBadge();

  return (
    <div className="bg-white dark:bg-slate-800/90 rounded-2xl border border-slate-200 dark:border-slate-700 p-5 shadow-sm hover:shadow-md transition duration-200 flex flex-col justify-between">
      <div>
        {/* Header: Name, Subtitle, Badges */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start space-x-3 min-w-0 flex-1">
            <div className="p-2.5 rounded-xl bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 shrink-0">
              <PrinterIcon className="w-6 h-6" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center space-x-2 flex-wrap gap-y-1">
                <h2
                  className="font-semibold text-slate-900 dark:text-white text-base truncate max-w-[190px]"
                  title={printer.name}
                >
                  {printer.name}
                </h2>
                {printer.is_default && (
                  <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900/60 dark:text-amber-300 border border-amber-200 dark:border-amber-700 shrink-0">
                    <Star className="w-3 h-3 fill-current" />
                    <span>Default</span>
                  </span>
                )}
                {printer.is_class && (
                  <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900/60 dark:text-purple-300 shrink-0">
                    Pool
                  </span>
                )}
              </div>
              <p
                className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 truncate"
                title={printer.info || printer.name}
              >
                {printer.info || printer.name}
              </p>
            </div>
          </div>

          {/* Live State Badge - shrink-0 ensures it is always docked in the top-right corner */}
          <span
            className={`inline-flex items-center space-x-1.5 px-2.5 py-1 rounded-full text-xs font-semibold shrink-0 border ${stateStyle.pill}`}
          >
            <span className={`w-2 h-2 rounded-full ${stateStyle.dot}`} />
            <span>{stateStyle.label}</span>
          </span>
        </div>

        {/* Metadata section */}
        <div className="mt-4 pt-3 border-t border-slate-100 dark:border-slate-700 space-y-2 text-xs text-slate-600 dark:text-slate-300">
          {/* Driver & Architecture Readiness */}
          {printer.arch_readiness && (
            <div className="flex items-center justify-between text-xs py-0.5">
              <div className="flex items-center space-x-1.5 text-slate-500 dark:text-slate-400 shrink-0">
                <Cpu className="w-3.5 h-3.5" />
                <span>Driver (PPD)</span>
              </div>
              <button
                type="button"
                onClick={() => onConfigure?.(printer, 'audit')}
                className={`inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-md text-xs font-medium border whitespace-nowrap cursor-pointer hover:opacity-85 transition ${
                  printer.arch_readiness.rosetta_required
                    ? 'bg-amber-50 text-amber-800 dark:bg-amber-950/60 dark:text-amber-300 border-amber-300 dark:border-amber-700/80 hover:bg-amber-100 dark:hover:bg-amber-900/50'
                    : printer.arch_readiness.status === 'driverless'
                    ? 'bg-sky-50 text-sky-800 dark:bg-sky-950/50 dark:text-sky-300 border-sky-200 dark:border-sky-800/70 hover:bg-sky-100 dark:hover:bg-sky-900/50'
                    : 'bg-emerald-50 text-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800/70 hover:bg-emerald-100 dark:hover:bg-emerald-900/50'
                }`}
                title="Click to view Rosetta 2 & Mach-O binary audit"
              >
                {printer.arch_readiness.rosetta_required ? (
                  <AlertTriangle className="w-3 h-3 text-amber-600 dark:text-amber-400 shrink-0" aria-hidden="true" />
                ) : printer.arch_readiness.status === 'driverless' ? (
                  <span className="w-1.5 h-1.5 rounded-full bg-sky-500 shrink-0" aria-hidden="true" />
                ) : (
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0" aria-hidden="true" />
                )}
                <span>{printer.arch_readiness.label}</span>
              </button>
            </div>
          )}
          {printer.location && (
            <div className="flex items-center space-x-2 text-slate-500 dark:text-slate-400">
              <MapPin className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
              <span className="truncate">{printer.location}</span>
            </div>
          )}

          {printer.uri && (
            <div className="flex items-center space-x-2 font-mono text-xs text-slate-500 dark:text-slate-400">
              <HardDrive className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
              <span className="truncate" title={printer.uri}>
                {printer.uri}
              </span>
            </div>
          )}

          {printer.state_message && printer.state_message !== 'Ready to print' && (
            <p className="text-xs text-slate-500 dark:text-slate-400 italic">
              {printer.state_message}
            </p>
          )}

          {/* Alerts / Warnings */}
          {printer.alerts && printer.alerts.length > 0 && (
            <div className="flex flex-wrap gap-1.5 pt-1">
              {printer.alerts.map((alert) => {
                const { label, severity } = formatAlert(alert);
                const color =
                  severity === 'error'
                    ? 'bg-rose-100 text-rose-800 dark:bg-rose-950/60 dark:text-rose-300 border-rose-200'
                    : 'bg-amber-100 text-amber-800 dark:bg-amber-950/60 dark:text-amber-300 border-amber-200';
                return (
                  <span
                    key={alert}
                    className={`inline-flex items-center space-x-1 px-2 py-0.5 rounded text-xs font-medium border ${color}`}
                  >
                    <AlertCircle className="w-3 h-3" aria-hidden="true" />
                    <span>{label}</span>
                  </span>
                );
              })}
            </div>
          )}
        </div>
      </div>

      {/* Footer & Actions */}
      <div className="mt-5 pt-3 border-t border-slate-100 dark:border-slate-700 flex items-center justify-between">
        <div className="flex items-center space-x-2 text-xs text-slate-500 dark:text-slate-400">
          <div className="flex items-center space-x-1.5">
            <FileText className="w-3.5 h-3.5" aria-hidden="true" />
            <span>
              {printer.queued_jobs_count === 1
                ? '1 job in queue'
                : `${printer.queued_jobs_count} jobs in queue`}
            </span>
          </div>

          {printer.queued_jobs_count > 0 && (
            confirmPurge ? (
              <div className="flex items-center space-x-1">
                <button
                  type="button"
                  onClick={handlePurge}
                  disabled={actionLoading}
                  className="px-1.5 py-0.5 rounded text-xs font-semibold bg-rose-600 text-white hover:bg-rose-700 transition cursor-pointer"
                  title="Confirm purging all jobs in this queue"
                >
                  Purge?
                </button>
                <button
                  type="button"
                  onClick={() => setConfirmPurge(false)}
                  disabled={actionLoading}
                  aria-label="Cancel purge"
                  className="px-1 py-0.5 rounded text-xs text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 cursor-pointer"
                >
                  ✕
                </button>
              </div>
            ) : (
              <button
                type="button"
                onClick={() => setConfirmPurge(true)}
                disabled={actionLoading}
                title="Purge all jobs in this queue"
                aria-label={`Purge all jobs in ${printer.name}`}
                className="p-1 rounded text-slate-500 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-950/40 transition cursor-pointer"
              >
                <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
              </button>
            )
          )}
        </div>

        <div className="flex items-center space-x-2">
          {onConfigure && (
            <button
              onClick={() => onConfigure(printer, 'preferences')}
              disabled={actionLoading}
              title="Configure PPD options & hardware accessories"
              className="inline-flex items-center space-x-1 px-2.5 py-1 rounded-lg text-xs font-medium text-slate-700 dark:text-slate-200 bg-slate-100 dark:bg-slate-700 hover:bg-slate-200 dark:hover:bg-slate-600 transition cursor-pointer"
            >
              <Sliders className="w-3 h-3 text-slate-500 dark:text-slate-400" />
              <span>Configure</span>
            </button>
          )}

          {!printer.is_default && (
            <button
              onClick={handleSetDefault}
              disabled={actionLoading}
              title="Set as system default printer"
              className="px-2.5 py-1 rounded-lg text-xs font-medium text-slate-700 dark:text-slate-200 bg-slate-100 dark:bg-slate-700 hover:bg-slate-200 dark:hover:bg-slate-600 transition cursor-pointer"
            >
              Make Default
            </button>
          )}

          <button
            onClick={handleToggleState}
            disabled={actionLoading}
            className={`inline-flex items-center space-x-1 px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
              printer.state === 'stopped'
                ? 'bg-emerald-600 hover:bg-emerald-700 text-white'
                : 'bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-200 hover:bg-slate-200 dark:hover:bg-slate-600'
            }`}
          >
            {printer.state === 'stopped' ? (
              <>
                <Play className="w-3 h-3" />
                <span>Resume</span>
              </>
            ) : (
              <>
                <Pause className="w-3 h-3" />
                <span>Pause</span>
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};
