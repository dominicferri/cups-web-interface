import React, { useState, useEffect, useRef } from 'react';
import {
  Printer as PrinterIcon,
  Star,
  Sliders,
  MoreVertical,
  Trash2,
  Search,
  CheckCircle2,
  AlertTriangle,
  RefreshCw,
  Plus,
  Layers,
  Network,
  FileText,
} from 'lucide-react';
import type { Printer } from '../types/cups';
import type { ConfigModalTab } from './PrinterConfigModal';
import { formatAlert } from '../utils/format';

interface PrinterTableProps {
  printers: Printer[];
  onPause: (name: string) => Promise<void>;
  onResume: (name: string) => Promise<void>;
  onSetDefault: (name: string) => Promise<void>;
  onPurge: (name: string) => Promise<void>;
  onConfigure: (printer: Printer, tab?: ConfigModalTab) => void;
  onDelete: (printer: Printer) => void;
  onAddPrinter: () => void;
  onOpenServerSettings: () => void;
  onPrintTestPage: (name: string) => Promise<void>;
}

export const PrinterTable: React.FC<PrinterTableProps> = ({
  printers,
  onPause,
  onResume,
  onSetDefault,
  onPurge,
  onConfigure,
  onDelete,
  onAddPrinter,
  onOpenServerSettings,
  onPrintTestPage,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [activeKebab, setActiveKebab] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close kebab menu when clicking outside or pressing Escape
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setActiveKebab(null);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setActiveKebab(null);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, []);

  const filteredPrinters = printers
    .filter((p) => {
      const q = searchQuery.toLowerCase();
      return (
        p.name.toLowerCase().includes(q) ||
        (p.info && p.info.toLowerCase().includes(q)) ||
        (p.location && p.location.toLowerCase().includes(q)) ||
        (p.uri && p.uri.toLowerCase().includes(q)) ||
        (p.arch_readiness && p.arch_readiness.label.toLowerCase().includes(q)) ||
        (p.arch_readiness?.driver_model && p.arch_readiness.driver_model.toLowerCase().includes(q))
      );
    })
    .sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));

  const isEnabledSince = (msg?: string) => {
    if (!msg) return false;
    const lower = msg.toLowerCase().trim();
    // These repeat what the status already says, so they are not shown again.
    return lower.startsWith('enabled since') || lower.startsWith('disabled since') || lower === 'ready to print';
  };

  const handleToggleState = async (printer: Printer) => {
    setActionLoading(printer.name);
    try {
      if (printer.state === 'stopped') {
        await onResume(printer.name);
      } else {
        await onPause(printer.name);
      }
    } finally {
      setActionLoading(null);
    }
  };

  const handleMakeDefault = async (printer: Printer) => {
    setActiveKebab(null);
    setActionLoading(printer.name);
    try {
      await onSetDefault(printer.name);
    } finally {
      setActionLoading(null);
    }
  };

  const handlePurge = async (printer: Printer) => {
    setActiveKebab(null);
    setActionLoading(printer.name);
    try {
      await onPurge(printer.name);
    } finally {
      setActionLoading(null);
    }
  };

  // Status is shown as a dot and a word. Green means healthy; red means the queue needs attention.
  const getStateBadge = (state: string) => {
    const healthy = 'bg-blue-600 dark:bg-blue-400';
    const problem = 'bg-red-600 dark:bg-red-400';
    switch (state) {
      case 'idle':
        return { dot: healthy, label: 'Ready' };
      case 'printing':
        return { dot: `${healthy} animate-pulse`, label: 'Printing' };
      case 'stopped':
        return { dot: problem, label: 'Paused' };
      case 'error':
      default:
        return { dot: problem, label: state === 'error' ? 'Error' : state };
    }
  };

  const getArchBadge = (printer: Printer) => {
    const plain = 'text-sm text-slate-600 dark:text-slate-300 group-hover:underline';
    if (!printer.arch_readiness) {
      return <span className={plain}>Driver (PPD)</span>;
    }

    const { status, label } = printer.arch_readiness;
    switch (status) {
      case 'universal':
      case 'native_arm64':
        return <span className={plain}>Universal</span>;
      case 'rosetta_dependent':
        return (
          <span className="inline-flex flex-col items-start text-left">
            <span className="inline-flex items-center gap-1.5 text-sm font-semibold text-amber-600 dark:text-amber-400 group-hover:underline">
              <AlertTriangle className="w-3.5 h-3.5 shrink-0" />
              <span>Intel only</span>
            </span>
            <span className="text-xs text-slate-500 dark:text-slate-400">Needs Rosetta 2</span>
          </span>
        );
      case 'driverless':
        return <span className={plain}>Driverless</span>;
      case 'missing_binary':
      default:
        return (
          <span className="text-sm font-semibold text-red-600 dark:text-red-400 group-hover:underline">
            {label || 'Missing binary'}
          </span>
        );
    }
  };

  return (
    <div className="space-y-4">
      {/* Search & Filter Toolbar */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3">
        <div className="relative flex-1 max-w-md">
          <label htmlFor="printer-search-input" className="sr-only">
            Search printer queues
          </label>
          <Search className="w-4 h-4 text-slate-500 dark:text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" aria-hidden="true" />
          <input
            id="printer-search-input"
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search by queue name, driver, location, or URI..."
            aria-label="Search printer queues"
            className="w-full h-9 pl-9 pr-4 rounded-md text-sm bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-hidden focus:ring-2 focus:ring-blue-600/40 focus:border-blue-600"
          />
        </div>

        <div className="flex items-center space-x-2.5">
          <button
            type="button"
            onClick={onOpenServerSettings}
            className="inline-flex items-center h-9 px-3.5 rounded-md text-sm font-semibold bg-white dark:bg-slate-900 text-slate-900 dark:text-white border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 transition-colors cursor-pointer whitespace-nowrap"
            title="Configure CUPS daemon parameters (cupsctl)"
          >
            <span>CUPS Settings</span>
          </button>

          <button
            type="button"
            onClick={onAddPrinter}
            className="inline-flex items-center gap-1.5 h-9 px-3.5 rounded-md text-sm font-semibold bg-blue-600 hover:bg-blue-700 text-white transition-colors cursor-pointer whitespace-nowrap"
            title="Create a new CUPS printer queue"
          >
            <Plus className="w-4 h-4" aria-hidden="true" />
            <span>Add Printer Queue</span>
          </button>
        </div>
      </div>

      {/* Main Table Container */}
      <div className="bg-white dark:bg-slate-800 rounded-md border border-slate-200 dark:border-slate-700 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="bg-slate-50 dark:bg-slate-900/60 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider border-b border-slate-200 dark:border-slate-700">
                <th className="py-3 px-4 sm:px-5">Queue</th>
                <th className="hidden sm:table-cell py-3 px-4">Status</th>
                <th className="hidden xl:table-cell py-3 px-4">Driver</th>
                <th className="hidden lg:table-cell py-3 px-4">Location &amp; Connection</th>
                <th className="hidden md:table-cell py-3 px-4 text-right">Jobs</th>
                <th className="py-3 px-4 sm:px-5 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
              {filteredPrinters.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-12 text-center text-slate-500 dark:text-slate-400">
                    <PrinterIcon className="w-8 h-8 mx-auto mb-2 opacity-40" aria-hidden="true" />
                    <p className="font-semibold text-sm text-slate-900 dark:text-white">
                      No matching printers found
                    </p>
                    <p className="text-xs mt-0.5">
                      Try adjusting your search query or clear the filter.
                    </p>
                  </td>
                </tr>
              ) : (
                filteredPrinters.map((printer) => {
                  const stateStyle = getStateBadge(printer.state);
                  const isMenuOpen = activeKebab === printer.name;

                  return (
                    <tr
                      key={printer.name}
                      className="hover:bg-slate-50/60 dark:hover:bg-slate-800/50 transition-colors"
                    >
                      {/* Queue / Printer Name Column: takes the remaining width and truncates,
                          so long queue names never push the Actions column out of view */}
                      <td className="py-3 px-4 sm:px-5 w-full max-w-0 align-top">
                        <div className="flex items-center space-x-3 min-w-[11rem]">
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center space-x-2 flex-wrap gap-y-1">
                              <span
                                className="font-semibold text-[15px] font-[family-name:var(--font-display)] text-slate-900 dark:text-white truncate min-w-0 max-w-full"
                                title={printer.name}
                              >
                                {printer.name}
                              </span>

                              {/* Compact Status Indicator for screens where Status column is hidden (< sm) */}
                              <span
                                className="sm:hidden inline-flex items-center gap-1.5 text-xs text-slate-600 dark:text-slate-300 shrink-0"
                              >
                                <span className={`w-2 h-2 rounded-full ${stateStyle.dot}`} aria-hidden="true" />
                                <span>{stateStyle.label}</span>
                              </span>

                              {printer.is_default && (
                                <span className="inline-flex items-center px-1.5 py-0.5 rounded-sm text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 border border-slate-200 dark:border-slate-700 shrink-0">
                                  Default
                                </span>
                              )}

                              {printer.is_class && (
                                <span className="inline-flex items-center px-1.5 py-0.5 rounded-sm text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 border border-slate-200 dark:border-slate-700 shrink-0">
                                  Pool
                                </span>
                              )}
                            </div>

                            <p
                              className="text-[13px] text-slate-500 dark:text-slate-400 truncate"
                              title={printer.info || printer.name}
                            >
                              {printer.info || printer.name}
                            </p>

                            {/* Alerts as one line of text: red for errors, orange for warnings */}
                            {printer.alerts && printer.alerts.length > 0 && (
                              <p className="text-[13px] mt-0.5 truncate">
                                {printer.alerts.map((alert, index) => {
                                  const { label, severity } = formatAlert(alert);
                                  return (
                                    <span
                                      key={alert}
                                      className={
                                        severity === 'error'
                                          ? 'text-red-600 dark:text-red-400'
                                          : 'text-amber-600 dark:text-amber-400'
                                      }
                                    >
                                      {index > 0 && <span className="text-slate-400"> · </span>}
                                      {label}
                                    </span>
                                  );
                                })}
                              </p>
                            )}
                          </div>
                        </div>
                      </td>

                      {/* Status Column: Hidden on < sm */}
                      <td className="hidden sm:table-cell py-3 px-4 whitespace-nowrap align-top">
                        <div className="flex items-center gap-2 text-sm text-slate-900 dark:text-white">
                          <span className={`w-2 h-2 rounded-full shrink-0 ${stateStyle.dot}`} />
                          <span>{stateStyle.label}</span>
                        </div>

                        {printer.diagnostic && (
                          <p
                            className="ml-4 text-[13px] text-red-600 dark:text-red-400 truncate max-w-[170px]"
                            title={`${printer.diagnostic.title}: ${printer.diagnostic.explanation}`}
                          >
                            {printer.diagnostic.title}
                          </p>
                        )}

                        {printer.state_message && !isEnabledSince(printer.state_message) && !printer.diagnostic && (
                          <p
                            className="ml-4 text-[13px] text-slate-500 dark:text-slate-400 max-w-[170px] truncate"
                            title={printer.state_message}
                          >
                            {printer.state_message}
                          </p>
                        )}
                      </td>

                      {/* Driver (PPD) & Arch Readiness: Hidden on < xl */}
                      <td className="hidden xl:table-cell py-3 px-4 whitespace-nowrap align-top">
                        <button
                          type="button"
                          onClick={() => onConfigure(printer, 'audit')}
                          className="group inline-flex items-center space-x-1 hover:opacity-85 transition cursor-pointer"
                          title="Click to view Mach-O architecture & driver audit"
                        >
                          {getArchBadge(printer)}
                        </button>
                      </td>

                      {/* Location & Connection Column: Hidden on < lg */}
                      <td className="hidden lg:table-cell py-3 px-4 align-top">
                        <div className="text-sm text-slate-600 dark:text-slate-300">
                          <span className="block truncate max-w-[240px]" title={printer.location || 'Not set'}>
                            {printer.location || 'Not set'}
                          </span>
                        </div>
                        <div
                          className="text-xs font-mono text-slate-500 dark:text-slate-400 truncate max-w-[240px]"
                          title={printer.uri}
                        >
                          {printer.uri}
                        </div>
                      </td>

                      {/* Jobs Column: Hidden on < md */}
                      <td className="hidden md:table-cell py-3 px-4 text-right whitespace-nowrap align-top tabular-nums text-sm">
                        <span
                          className={
                            printer.queued_jobs_count > 0
                              ? 'font-semibold text-slate-900 dark:text-white'
                              : 'text-slate-500 dark:text-slate-400'
                          }
                          title={`${printer.queued_jobs_count} ${printer.queued_jobs_count === 1 ? 'job' : 'jobs'} queued`}
                        >
                          {printer.queued_jobs_count}
                        </span>
                      </td>

                      {/* Actions Column: Always visible */}
                      <td className="py-2.5 px-4 sm:px-5 text-right whitespace-nowrap align-top">
                        <div className="inline-flex items-center space-x-2">
                          {/* Inline Pause / Resume Toggle */}
                          <button
                            type="button"
                            onClick={() => handleToggleState(printer)}
                            disabled={actionLoading === printer.name}
                            className={`inline-flex items-center justify-center h-8 min-w-[4.5rem] px-3 rounded-md text-[13px] font-semibold transition-colors cursor-pointer ${
                              printer.state === 'stopped'
                                ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-white border border-slate-200 dark:border-slate-700 hover:border-blue-600 dark:hover:border-blue-400'
                                : 'text-blue-700 dark:text-blue-400 hover:bg-slate-100 dark:hover:bg-slate-700/60'
                            }`}
                            title={printer.state === 'stopped' ? 'Resume printer queue' : 'Pause printer queue'}
                          >
                            {printer.state === 'stopped' ? 'Resume' : 'Pause'}
                          </button>

                          {/* Kebab Menu Container */}
                          <div className="relative inline-block text-left" ref={isMenuOpen ? menuRef : undefined}>
                            <button
                              type="button"
                              onClick={() => setActiveKebab(isMenuOpen ? null : printer.name)}
                              className="p-2 rounded-md text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700 transition cursor-pointer"
                              title="More options"
                              aria-label={`More options for ${printer.name}`}
                              aria-haspopup="menu"
                              aria-expanded={isMenuOpen}
                            >
                              <MoreVertical className="w-4 h-4" aria-hidden="true" />
                            </button>

                            {/* Dropdown Popover */}
                            {isMenuOpen && (
                              <div
                                role="menu"
                                aria-label={`Actions for ${printer.name}`}
                                className="origin-top-right absolute right-0 mt-2 w-52 rounded-2xl shadow-xl bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 py-1.5 z-40 animate-in fade-in zoom-in-95 duration-100"
                              >
                                <div className="px-3 py-1.5 border-b border-slate-100 dark:border-slate-700 mb-1">
                                  <p className="text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider truncate">
                                    Queue: {printer.name}
                                  </p>
                                </div>

                                {/* Print Test Page Option */}
                                <button
                                  type="button"
                                  role="menuitem"
                                  onClick={() => {
                                    setActiveKebab(null);
                                    onPrintTestPage(printer.name);
                                  }}
                                  className="w-full text-left px-3.5 py-2 text-xs font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700/70 flex items-center space-x-2.5 transition cursor-pointer"
                                >
                                  <FileText className="w-3.5 h-3.5 text-blue-600 dark:text-blue-400" aria-hidden="true" />
                                  <span>Print Test Page</span>
                                </button>

                                {/* Default Options */}
                                <button
                                  type="button"
                                  role="menuitem"
                                  onClick={() => {
                                    setActiveKebab(null);
                                    onConfigure(printer, 'preferences');
                                  }}
                                  className="w-full text-left px-3.5 py-2 text-xs font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700/70 flex items-center space-x-2.5 transition cursor-pointer"
                                >
                                  <Sliders className="w-3.5 h-3.5 text-slate-500 dark:text-slate-400" aria-hidden="true" />
                                  <span>Default Options...</span>
                                </button>

                                {/* Queue Details Option */}
                                <button
                                  type="button"
                                  role="menuitem"
                                  onClick={() => {
                                    setActiveKebab(null);
                                    onConfigure(printer, 'details');
                                  }}
                                  className="w-full text-left px-3.5 py-2 text-xs font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700/70 flex items-center space-x-2.5 transition cursor-pointer"
                                >
                                  <Network className="w-3.5 h-3.5 text-slate-500 dark:text-slate-400" aria-hidden="true" />
                                  <span>Queue Details &amp; URI...</span>
                                </button>

                                {/* Swap Driver Option */}
                                <button
                                  type="button"
                                  role="menuitem"
                                  onClick={() => {
                                    setActiveKebab(null);
                                    onConfigure(printer, 'driver');
                                  }}
                                  className="w-full text-left px-3.5 py-2 text-xs font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700/70 flex items-center space-x-2.5 transition cursor-pointer"
                                >
                                  <Layers className="w-3.5 h-3.5 text-slate-500 dark:text-slate-400" aria-hidden="true" />
                                  <span>Swap Driver / PPD...</span>
                                </button>

                                {/* Make Default Option */}
                                {!printer.is_default ? (
                                  <button
                                    type="button"
                                    role="menuitem"
                                    onClick={() => handleMakeDefault(printer)}
                                    className="w-full text-left px-3.5 py-2 text-xs font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700/70 flex items-center space-x-2.5 transition cursor-pointer"
                                  >
                                    <Star className="w-3.5 h-3.5 text-amber-500" aria-hidden="true" />
                                    <span>Make Default</span>
                                  </button>
                                ) : (
                                  <div className="w-full text-left px-3.5 py-2 text-xs font-medium text-slate-500 dark:text-slate-400 flex items-center space-x-2.5 cursor-default">
                                    <CheckCircle2 className="w-3.5 h-3.5 text-blue-500" aria-hidden="true" />
                                    <span>Current Default</span>
                                  </div>
                                )}

                                {/* Purge Jobs (if queue has jobs) */}
                                {printer.queued_jobs_count > 0 && (
                                  <button
                                    type="button"
                                    role="menuitem"
                                    onClick={() => handlePurge(printer)}
                                    className="w-full text-left px-3.5 py-2 text-xs font-medium text-amber-700 dark:text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-950/40 flex items-center space-x-2.5 transition cursor-pointer"
                                  >
                                    <RefreshCw className="w-3.5 h-3.5 text-amber-600" aria-hidden="true" />
                                    <span>Purge All Jobs ({printer.queued_jobs_count})</span>
                                  </button>
                                )}

                                <div className="border-t border-slate-100 dark:border-slate-700 my-1" />

                                {/* Delete Queue Option */}
                                <button
                                  type="button"
                                  role="menuitem"
                                  onClick={() => {
                                    setActiveKebab(null);
                                    onDelete(printer);
                                  }}
                                  className="w-full text-left px-3.5 py-2 text-xs font-semibold text-rose-600 dark:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-950/40 flex items-center space-x-2.5 transition cursor-pointer"
                                >
                                  <Trash2 className="w-3.5 h-3.5 text-rose-600 dark:text-rose-400" aria-hidden="true" />
                                  <span>Delete Queue...</span>
                                </button>
                              </div>
                            )}
                          </div>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
