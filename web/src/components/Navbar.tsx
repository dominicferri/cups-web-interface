import React from 'react';
import { Printer as PrinterIcon, RefreshCw, Moon, Sun, AlertTriangle } from 'lucide-react';
import type { SystemStatus } from '../types/cups';

interface NavbarProps {
  status: SystemStatus | null;
  loading: boolean;
  onRefresh: () => void;
  darkMode: boolean;
  onToggleDarkMode: () => void;
  onOpenFleetAudit?: () => void;
  rosettaRiskCount?: number;
}

export const Navbar: React.FC<NavbarProps> = ({
  status,
  loading,
  onRefresh,
  darkMode,
  onToggleDarkMode,
  onOpenFleetAudit,
  rosettaRiskCount = 0,
}) => {
  const iconButton =
    'p-2 rounded-md text-slate-300 hover:text-white hover:bg-white/10 transition-colors cursor-pointer';

  return (
    <header className="sticky top-0 z-40 w-full bg-slate-900 dark:bg-slate-950 border-b border-slate-900 dark:border-slate-700 text-white">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-14 flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-8 h-8 rounded-md bg-blue-600 text-white flex items-center justify-center shrink-0">
            <PrinterIcon className="w-5 h-5" aria-hidden="true" />
          </div>
          <h1 className="text-lg font-semibold text-white truncate">Web interface for CUPS</h1>
          <span className="hidden sm:inline text-xs text-slate-400">v0.1.0</span>
        </div>

        <div className="flex items-center gap-1 sm:gap-2 shrink-0">
          {status && (
            <div className="hidden md:flex items-center gap-2 text-sm text-slate-300 pr-3">
              <span
                className={`w-2 h-2 rounded-full ${status.daemon_running ? 'bg-blue-400' : 'bg-red-400'}`}
                aria-hidden="true"
              />
              <span>
                CUPS {status.cups_version} {status.daemon_running ? 'running' : 'offline'}
              </span>
            </div>
          )}

          {onOpenFleetAudit && (
            <button
              type="button"
              onClick={onOpenFleetAudit}
              title="Rosetta 2 & Apple Silicon Fleet Migration Audit"
              className={`hidden sm:inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium hover:bg-white/10 transition-colors cursor-pointer md:border-l md:border-slate-700 md:rounded-none md:pl-4 ${
                rosettaRiskCount > 0 ? 'text-amber-400' : 'text-slate-300'
              }`}
            >
              {rosettaRiskCount > 0 && <AlertTriangle className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />}
              <span>
                {rosettaRiskCount > 0
                  ? `${rosettaRiskCount} ${rosettaRiskCount === 1 ? 'driver needs' : 'drivers need'} Rosetta`
                  : 'Rosetta audit'}
              </span>
            </button>
          )}

          <button
            type="button"
            onClick={onRefresh}
            disabled={loading}
            title="Refresh printers and jobs"
            aria-label="Refresh printers and jobs"
            className={iconButton}
          >
            <RefreshCw className={`w-[18px] h-[18px] ${loading ? 'animate-spin' : ''}`} aria-hidden="true" />
          </button>

          <button
            type="button"
            onClick={onToggleDarkMode}
            title={darkMode ? 'Switch to light mode' : 'Switch to dark mode'}
            aria-label={darkMode ? 'Switch to light mode' : 'Switch to dark mode'}
            className={iconButton}
          >
            {darkMode ? (
              <Sun className="w-[18px] h-[18px]" aria-hidden="true" />
            ) : (
              <Moon className="w-[18px] h-[18px]" aria-hidden="true" />
            )}
          </button>
        </div>
      </div>
    </header>
  );
};
