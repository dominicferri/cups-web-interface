import { useState, useEffect, useCallback } from 'react';
import { api } from './api/client';
import type { Printer, Job, SystemStatus } from './types/cups';
import { Navbar } from './components/Navbar';
import { PrinterTable } from './components/PrinterTable';
import { JobQueueTable } from './components/JobQueueTable';
import { StuckJobsBanner } from './components/StuckJobsBanner';
import { PrinterConfigModal, type ConfigModalTab } from './components/PrinterConfigModal';
import { DeletePrinterModal } from './components/DeletePrinterModal';
import { FleetAuditModal } from './components/FleetAuditModal';
import { AddPrinterModal } from './components/AddPrinterModal';
import { ServerSettingsModal } from './components/ServerSettingsModal';
import { LogsViewer } from './components/LogsViewer';
import {
  Printer as PrinterIcon,
  CheckCircle2,
  AlertCircle,
} from 'lucide-react';

export function App() {
  const [printers, setPrinters] = useState<Printer[]>([]);
  const [activeJobs, setActiveJobs] = useState<Job[]>([]);
  const [completedJobs, setCompletedJobs] = useState<Job[]>([]);
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Top-level navigation tab: 'printers' | 'jobs' | 'logs'
  const [activeTab, setActiveTab] = useState<'printers' | 'jobs' | 'logs'>('printers');

  // Modal states for PPD configuration & Rosetta 2 audit
  const [configModalPrinter, setConfigModalPrinter] = useState<Printer | null>(null);
  const [configModalTab, setConfigModalTab] = useState<ConfigModalTab>('preferences');
  const [isConfigModalOpen, setIsConfigModalOpen] = useState(false);
  const [isFleetAuditOpen, setIsFleetAuditOpen] = useState(false);

  // Phase 4 modals: Add Printer & Server Settings
  const [isAddPrinterOpen, setIsAddPrinterOpen] = useState(false);
  const [isServerSettingsOpen, setIsServerSettingsOpen] = useState(false);

  // Delete printer modal state
  const [deleteModalPrinter, setDeleteModalPrinter] = useState<Printer | null>(null);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);

  // Dark mode (defaults to light mode)
  // Follow the system appearance until the user picks a theme with the toggle.
  const [darkMode, setDarkMode] = useState<boolean>(() => {
    const stored = localStorage.getItem('theme');
    if (stored === 'dark' || stored === 'light') {
      return stored === 'dark';
    }
    return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false;
  });

  useEffect(() => {
    document.documentElement.classList.toggle('dark', darkMode);
  }, [darkMode]);

  const toggleDarkMode = () => {
    const next = !darkMode;
    localStorage.setItem('theme', next ? 'dark' : 'light');
    setDarkMode(next);
  };

  const showToast = (message: string, type: 'success' | 'error' = 'success') => {
    setToast({ message, type });
    setTimeout(() => {
      setToast(null);
    }, 3500);
  };

  const fetchData = useCallback(async (isInitial = false) => {
    if (isInitial) setLoading(true);
    try {
      const [sysStatus, prs, actJobs, compJobs] = await Promise.all([
        api.getSystemStatus(),
        api.getPrinters(),
        api.getJobs('active'),
        api.getJobs('completed'),
      ]);
      setStatus(sysStatus);
      const sortedPrs = [...prs].sort((a, b) =>
        a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
      );
      setPrinters(sortedPrs);
      setActiveJobs(actJobs);
      setCompletedJobs(compJobs);
      setError(null);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to communicate with CUPS backend';
      setError(msg);
    } finally {
      if (isInitial) setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData(true);

    // Real-time updates via Server-Sent Events (SSE)
    const unsubscribe = api.subscribeEvents((event) => {
      if (event.type === 'job_update' || event.type === 'printer_update') {
        fetchData(false);
      }
    });

    // 5-second polling fallback
    const interval = setInterval(() => {
      fetchData(false);
    }, 5000);

    return () => {
      unsubscribe();
      clearInterval(interval);
    };
  }, [fetchData]);

  // Printer Actions
  const handlePausePrinter = async (name: string) => {
    try {
      await api.pausePrinter(name);
      showToast(`Printer "${name}" paused`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to pause printer';
      showToast(msg, 'error');
    }
  };

  const handleResumePrinter = async (name: string) => {
    try {
      await api.resumePrinter(name);
      showToast(`Printer "${name}" resumed`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to resume printer';
      showToast(msg, 'error');
    }
  };

  const handleSetDefaultPrinter = async (name: string) => {
    // Optimistically update printer state immediately
    setPrinters((prev) =>
      prev
        .map((p) => ({
          ...p,
          is_default: p.name === name,
        }))
        .sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
    );
    try {
      await api.setDefaultPrinter(name);
      showToast(`Printer "${name}" set as default`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to set default printer';
      showToast(msg, 'error');
      await fetchData(false);
    }
  };

  const handlePurgePrinter = async (name: string) => {
    try {
      await api.purgePrinter(name);
      showToast(`Queue "${name}" purged`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to purge queue';
      showToast(msg, 'error');
    }
  };

  const handleDeletePrinter = async (name: string) => {
    try {
      await api.deletePrinter(name);
      showToast(`Printer queue "${name}" deleted`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to delete printer queue';
      showToast(msg, 'error');
      throw err;
    }
  };

  // Job Actions
  const handleCancelJob = async (id: string) => {
    try {
      await api.cancelJob(id);
      showToast(`Print job #${id} cancelled`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to cancel job';
      showToast(msg, 'error');
    }
  };

  const handleHoldJob = async (id: string) => {
    try {
      await api.holdJob(id);
      showToast(`Print job #${id} held indefinitely`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to hold job';
      showToast(msg, 'error');
    }
  };

  const handleReleaseJob = async (id: string) => {
    try {
      await api.releaseJob(id);
      showToast(`Print job #${id} released`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to release job';
      showToast(msg, 'error');
    }
  };

  const handleRestartJob = async (id: string) => {
    try {
      await api.restartJob(id);
      showToast(`Print job #${id} restarted for printing`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to restart job';
      showToast(msg, 'error');
    }
  };

  const handlePrintTestPage = async (printerName: string) => {
    try {
      const jobId = await api.printTestPage(printerName);
      showToast(`Diagnostic test page sent to "${printerName}" (CUPS Job #${jobId})`);
      await fetchData(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to submit test page';
      showToast(msg, 'error');
    }
  };

  const stuckJobs = activeJobs.filter((j) => j.is_stuck);
  const rosettaRiskCount = printers.filter((p) => p.arch_readiness?.rosetta_required).length;
  const diagnosticIssueCount =
    printers.filter((p) => p.diagnostic || p.state === 'error' || p.state === 'stopped').length +
    activeJobs.filter((j) => j.diagnostic || j.is_stuck).length;

  return (
    <div className="min-h-screen flex flex-col bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-100 transition-colors">
      <Navbar
        status={status}
        loading={loading}
        onRefresh={() => fetchData(false)}
        darkMode={darkMode}
        onToggleDarkMode={toggleDarkMode}
        onOpenFleetAudit={() => setIsFleetAuditOpen(true)}
        rosettaRiskCount={rosettaRiskCount}
      />

      {/* Floating Toast Notification */}
      <div
        role={toast?.type === 'error' ? 'alert' : 'status'}
        aria-live={toast?.type === 'error' ? 'assertive' : 'polite'}
        aria-atomic="true"
        className="fixed bottom-6 right-6 z-50"
      >
        {toast && (
          <div
            className={`flex items-center space-x-2 px-4 py-2.5 rounded-xl shadow-lg border text-xs font-medium animate-fade-in ${
              toast.type === 'success'
                ? 'bg-emerald-600 text-white border-emerald-500'
                : 'bg-rose-600 text-white border-rose-500'
            }`}
          >
            {toast.type === 'success' ? (
              <CheckCircle2 className="w-4 h-4 shrink-0" aria-hidden="true" />
            ) : (
              <AlertCircle className="w-4 h-4 shrink-0" aria-hidden="true" />
            )}
            <span>{toast.message}</span>
          </div>
        )}
      </div>

      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-8">
        {/* Error Banner */}
        {error && (
          <div
            role="alert"
            className="p-4 rounded-xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-900 text-rose-800 dark:text-rose-300 text-xs flex items-center space-x-3"
          >
            <AlertCircle className="w-5 h-5 shrink-0 text-rose-600" aria-hidden="true" />
            <div>
              <span className="font-semibold">Backend Communication Error:</span> {error}
            </div>
          </div>
        )}

        {/* Stuck Jobs Alert (Persistent Notification if any job is stuck) */}
        {stuckJobs.length > 0 && (
          <StuckJobsBanner stuckJobs={stuckJobs} onCancelJob={handleCancelJob} />
        )}

        {/* Summary strip */}
        {(() => {
          const pausedCount = printers.filter((p) => p.state === 'stopped').length;
          const cell = 'px-5 py-4 min-w-0';
          const label = 'text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400';
          const value = 'font-semibold text-slate-900 dark:text-white mt-1 truncate';
          const hint = 'text-xs text-slate-500 dark:text-slate-400';
          const attention = 'text-xs font-semibold text-amber-600 dark:text-amber-400';
          return (
            <div className="grid grid-cols-2 lg:grid-cols-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-md divide-slate-200 dark:divide-slate-700 max-lg:[&>*:nth-child(n+3)]:border-t max-lg:[&>*:nth-child(even)]:border-l lg:divide-x [&>*]:border-slate-200 dark:[&>*]:border-slate-700">
              <div className={cell}>
                <h2 className={label}>Print queues</h2>
                <p className={`${value} text-2xl`}>{printers.length}</p>
                <p className={pausedCount > 0 ? attention : hint}>
                  {pausedCount > 0 ? `${pausedCount} paused` : 'All accepting jobs'}
                </p>
              </div>
              <div className={cell}>
                <h2 className={label}>Active jobs</h2>
                <p className={`${value} text-2xl`}>{activeJobs.length}</p>
                <p className={stuckJobs.length > 0 ? attention : hint}>
                  {stuckJobs.length > 0 ? `${stuckJobs.length} stuck` : 'None stuck'}
                </p>
              </div>
              <div className={cell}>
                <h2 className={label}>Default printer</h2>
                <p className={`${value} text-base leading-8`} title={status?.default_printer || 'None'}>
                  {status?.default_printer || 'Not set'}
                </p>
                <p className={hint}>Used when no printer is chosen</p>
              </div>
              <div className={cell}>
                <h2 className={label}>Print system</h2>
                <p className={`${value} text-base leading-8`}>CUPS {status?.cups_version || '2.x'}</p>
                <p className={status && !status.daemon_running ? 'text-xs font-semibold text-red-600 dark:text-red-400' : hint}>
                  {status ? (status.daemon_running ? 'Running' : 'Not running') : 'Checking…'}
                </p>
              </div>
            </div>
          );
        })()}

        {/* Top-Level Navigation Tabs */}
        {(() => {
          const tabs: Array<'printers' | 'jobs' | 'logs'> = ['printers', 'jobs', 'logs'];
          const handleTabKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
            const idx = tabs.indexOf(activeTab);
            let nextIdx = idx;
            if (e.key === 'ArrowRight') {
              nextIdx = (idx + 1) % tabs.length;
            } else if (e.key === 'ArrowLeft') {
              nextIdx = (idx - 1 + tabs.length) % tabs.length;
            } else if (e.key === 'Home') {
              nextIdx = 0;
            } else if (e.key === 'End') {
              nextIdx = tabs.length - 1;
            } else {
              return;
            }
            e.preventDefault();
            const nextTab = tabs[nextIdx];
            setActiveTab(nextTab);
            document.getElementById(`tab-${nextTab}`)?.focus();
          };
          const tab = (active: boolean) =>
            `inline-flex items-center gap-2 pt-2 pb-2.5 text-[15px] font-semibold border-b-[3px] -mb-px transition-colors cursor-pointer whitespace-nowrap ${
              active
                ? 'text-slate-900 dark:text-white border-blue-600 dark:border-blue-400'
                : 'text-slate-500 dark:text-slate-400 border-transparent hover:text-slate-900 dark:hover:text-white'
            }`;
          const count = 'text-sm font-normal text-slate-500 dark:text-slate-400';
          const problem = 'text-sm font-semibold text-red-600 dark:text-red-400';
          return (
            <div
              role="tablist"
              aria-label="Dashboard sections"
              onKeyDown={handleTabKeyDown}
              className="flex items-center gap-7 border-b border-slate-200 dark:border-slate-700 overflow-x-auto"
            >
              <button
                type="button"
                role="tab"
                id="tab-printers"
                aria-selected={activeTab === 'printers'}
                aria-controls="tabpanel-printers"
                tabIndex={activeTab === 'printers' ? 0 : -1}
                onClick={() => setActiveTab('printers')}
                className={tab(activeTab === 'printers')}
              >
                <span>Printers &amp; Queues</span>
                <span className={count}>{printers.length}</span>
              </button>
              <button
                type="button"
                role="tab"
                id="tab-jobs"
                aria-selected={activeTab === 'jobs'}
                aria-controls="tabpanel-jobs"
                tabIndex={activeTab === 'jobs' ? 0 : -1}
                onClick={() => setActiveTab('jobs')}
                className={tab(activeTab === 'jobs')}
              >
                <span>Print Jobs</span>
                {stuckJobs.length > 0 ? (
                  <span className={problem} title={`${stuckJobs.length} stuck job(s)`}>
                    {stuckJobs.length} stuck
                  </span>
                ) : (
                  <span className={count}>{activeJobs.length}</span>
                )}
              </button>
              <button
                type="button"
                role="tab"
                id="tab-logs"
                aria-selected={activeTab === 'logs'}
                aria-controls="tabpanel-logs"
                tabIndex={activeTab === 'logs' ? 0 : -1}
                onClick={() => setActiveTab('logs')}
                className={tab(activeTab === 'logs')}
              >
                <span>Logs &amp; Diagnostics</span>
                {diagnosticIssueCount > 0 && (
                  <span className={problem}>
                    {diagnosticIssueCount} {diagnosticIssueCount === 1 ? 'issue' : 'issues'}
                  </span>
                )}
              </button>
            </div>
          );
        })()}

        {/* Tab 1: Printers & Queues Table View */}
        {activeTab === 'printers' && (
          <div id="tabpanel-printers" role="tabpanel" aria-labelledby="tab-printers" className="space-y-4">
            {rosettaRiskCount > 0 && (
              <p className="text-sm text-slate-600 dark:text-slate-300">
                <span className="font-semibold text-amber-600 dark:text-amber-400">
                  {rosettaRiskCount} {rosettaRiskCount === 1 ? 'queue relies' : 'queues rely'} on Rosetta 2.
                </span>{' '}
                <button
                  type="button"
                  onClick={() => setIsFleetAuditOpen(true)}
                  className="font-semibold text-blue-700 dark:text-blue-400 hover:underline cursor-pointer"
                >
                  View the driver audit
                </button>
              </p>
            )}

            {loading && printers.length === 0 ? (
              <div className="h-64 rounded-2xl bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 p-6 animate-pulse" />
            ) : printers.length === 0 ? (
              <div className="bg-white dark:bg-slate-800/90 rounded-2xl border border-slate-200 dark:border-slate-700 p-12 text-center text-slate-500 dark:text-slate-400">
                <PrinterIcon className="w-10 h-10 mx-auto mb-3 opacity-40" aria-hidden="true" />
                <p className="font-semibold text-base text-slate-900 dark:text-white">
                  No Printers Configured
                </p>
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                  CUPS does not currently have any active printer destinations registered.
                </p>
              </div>
            ) : (
              <PrinterTable
                printers={printers}
                onPause={handlePausePrinter}
                onResume={handleResumePrinter}
                onSetDefault={handleSetDefaultPrinter}
                onPurge={handlePurgePrinter}
                onConfigure={(p, tab) => {
                  setConfigModalPrinter(p);
                  setConfigModalTab(tab || 'preferences');
                  setIsConfigModalOpen(true);
                }}
                onDelete={(p) => {
                  setDeleteModalPrinter(p);
                  setIsDeleteModalOpen(true);
                }}
                onAddPrinter={() => setIsAddPrinterOpen(true)}
                onOpenServerSettings={() => setIsServerSettingsOpen(true)}
                onPrintTestPage={handlePrintTestPage}
              />
            )}
          </div>
        )}

        {/* Tab 2: Print Jobs & Document Queue */}
        {activeTab === 'jobs' && (
          <div id="tabpanel-jobs" role="tabpanel" aria-labelledby="tab-jobs" className="space-y-4">
            <JobQueueTable
              activeJobs={activeJobs}
              completedJobs={completedJobs}
              onCancelJob={handleCancelJob}
              onHoldJob={handleHoldJob}
              onReleaseJob={handleReleaseJob}
              onRestartJob={handleRestartJob}
            />
          </div>
        )}

        {/* Tab 3: System Logs & Intelligent Diagnostics Engine */}
        {activeTab === 'logs' && (
          <div id="tabpanel-logs" role="tabpanel" aria-labelledby="tab-logs">
            <LogsViewer />
          </div>
        )}

        {/* Slide-over Drawer for PPD Configuration, Details, Driver & Rosetta 2 Audit */}
        <PrinterConfigModal
          printer={configModalPrinter}
          initialTab={configModalTab}
          isOpen={isConfigModalOpen}
          onClose={() => setIsConfigModalOpen(false)}
          onSaveSuccess={() => fetchData(false)}
        />

        {/* Phase 4: Add Printer Modal (Right-side slide drawer with Safe Naming) */}
        <AddPrinterModal
          isOpen={isAddPrinterOpen}
          onClose={() => setIsAddPrinterOpen(false)}
          onSuccess={(newName) => {
            showToast(`Print queue "${newName}" created successfully!`);
            fetchData(false);
          }}
          onConfigureCreated={(printer) => {
            setConfigModalPrinter(printer);
            setConfigModalTab('preferences');
            setIsConfigModalOpen(true);
            fetchData(false);
          }}
        />

        {/* Phase 4: CUPS Server Settings Modal (cupsctl) */}
        <ServerSettingsModal
          isOpen={isServerSettingsOpen}
          onClose={() => setIsServerSettingsOpen(false)}
          onSuccess={() => {
            showToast('CUPS server daemon settings applied successfully');
            fetchData(false);
          }}
        />

        {/* Delete Confirmation Modal */}
        <DeletePrinterModal
          printer={deleteModalPrinter}
          isOpen={isDeleteModalOpen}
          onClose={() => setIsDeleteModalOpen(false)}
          onConfirmDelete={handleDeletePrinter}
        />

        {/* Fleet Audit Modal */}
        <FleetAuditModal
          printers={printers}
          isOpen={isFleetAuditOpen}
          onClose={() => setIsFleetAuditOpen(false)}
          onSelectPrinterForAudit={(printer) => {
            setConfigModalPrinter(printer);
            setConfigModalTab('audit');
            setIsConfigModalOpen(true);
          }}
        />
      </main>

      {/* Footer */}
      <footer className="mt-auto border-t border-slate-200 dark:border-slate-800 bg-white/50 dark:bg-slate-900/50 py-6 text-xs text-slate-500 dark:text-slate-400">
        <div className="max-w-7xl mx-auto px-4 space-y-3">
          <div className="flex flex-col sm:flex-row items-center justify-between gap-2">
            <p>
              &copy; Copyright 1999-2026. PaperCut Software Pty Ltd. • Released under the{' '}
              <a
                href="https://opensource.org/licenses/MIT"
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-slate-700 dark:hover:text-slate-200 transition-colors"
              >
                MIT Open Source License
              </a>
            </p>
            <p>
              Bound strictly to loopback{' '}
              <code className="font-mono bg-slate-100 dark:bg-slate-800 px-1.5 py-0.5 rounded">127.0.0.1:6310</code> • Designed for macOS &amp; UNIX
            </p>
          </div>
          <div className="pt-3 border-t border-slate-200 dark:border-slate-800 text-xs leading-relaxed text-slate-500 dark:text-slate-400 text-center">
            Copyright &copy; 2007-2022 Apple Inc. CUPS 2.2 and earlier are provided under the terms of the{' '}
            <a
              href="https://www.cups.org/doc/old-license.html"
              target="_blank"
              rel="noopener noreferrer"
              className="underline hover:text-slate-700 dark:hover:text-slate-200 transition-colors"
            >
              GNU GPL2 and LGPL2 with exceptions
            </a>{' '}
            while CUPS 2.3 and later are provided under the terms of the{' '}
            <a
              href="https://www.cups.org/doc/license.html"
              target="_blank"
              rel="noopener noreferrer"
              className="underline hover:text-slate-700 dark:hover:text-slate-200 transition-colors"
            >
              Apache License, Version 2.0
            </a>
            . CUPS, the CUPS logo, and macOS are trademarks of{' '}
            <a
              href="https://www.apple.com/legal/intellectual-property/trademark/appletmlist.html"
              target="_blank"
              rel="noopener noreferrer"
              className="underline hover:text-slate-700 dark:hover:text-slate-200 transition-colors"
            >
              Apple Inc.
            </a>{' '}
            All other trademarks are the property of their respective owners.{' '}
            <a
              href="https://www.apple.com/legal/privacy/en-ww/"
              target="_blank"
              rel="noopener noreferrer"
              className="underline hover:text-slate-700 dark:hover:text-slate-200 transition-colors"
            >
              Apple Privacy Policy
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}

export default App;
