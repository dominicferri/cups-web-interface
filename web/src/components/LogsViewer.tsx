import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { apiClient } from '../api/client';
import type { LogEntry } from '../types/cups';
import {
  AlertTriangle,
  XCircle,
  AlertCircle,
  CheckCircle2,
  Info,
  RefreshCw,
  Search,
  Check,
  Copy,
  Wrench,
  ChevronDown,
  ChevronUp,
  Terminal,
  X,
} from 'lucide-react';

export const LogsViewer: React.FC = () => {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [levelFilter, setLevelFilter] = useState<string>('all');
  const [lineCount, setLineCount] = useState<number>(100);
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [autoRefresh, setAutoRefresh] = useState<boolean>(true);
  const [copied, setCopied] = useState<boolean>(false);
  const [copiedCmd, setCopiedCmd] = useState<string | null>(null);
  const [expandedDiagnostics, setExpandedDiagnostics] = useState<Record<number, boolean>>({});

  const fetchLogs = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await apiClient.getLogs(lineCount, levelFilter);
      setLogs(data);
    } catch (err: any) {
      setError(err?.message || 'Failed to fetch logs');
    } finally {
      setLoading(false);
    }
  }, [lineCount, levelFilter]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(() => {
      fetchLogs();
    }, 5000);
    return () => clearInterval(interval);
  }, [autoRefresh, fetchLogs]);

  const toggleDiagnostic = (index: number) => {
    setExpandedDiagnostics((prev) => ({
      ...prev,
      [index]: !prev[index],
    }));
  };

  const filteredLogs = useMemo(() => {
    let result = logs;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      result = result.filter(
        (entry) =>
          entry.message.toLowerCase().includes(q) ||
          (entry.printer && entry.printer.toLowerCase().includes(q)) ||
          (entry.job_id && entry.job_id.toLowerCase().includes(q)) ||
          (entry.diagnostic &&
            (entry.diagnostic.title.toLowerCase().includes(q) ||
              entry.diagnostic.code.toLowerCase().includes(q) ||
              entry.diagnostic.explanation.toLowerCase().includes(q) ||
              entry.diagnostic.remedies.some((r) => r.toLowerCase().includes(q))))
      );
    }
    return [...result].reverse();
  }, [logs, searchQuery]);

  const handleCopy = () => {
    const text = filteredLogs
      .map(
        (l) =>
          `[${l.raw_date}] [${l.level.toUpperCase()}] ${l.printer ? `[${l.printer}] ` : ''}${l.job_id ? `[Job ${l.job_id}] ` : ''}${l.message}`
      )
      .join('\n');
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleCopyCommand = (cmd: string) => {
    navigator.clipboard.writeText(cmd);
    setCopiedCmd(cmd);
    setTimeout(() => setCopiedCmd(null), 2000);
  };

  // Compute log health statistics from currently loaded logs
  const stats = useMemo(() => {
    let errors = 0;
    let warnings = 0;
    let diagnostics = 0;
    const categoryCounts: Record<string, { title: string; count: number; severity: 'error' | 'warning' }> = {};

    logs.forEach((l) => {
      if (l.level === 'error') errors++;
      if (l.level === 'warn' || l.level === 'warning') warnings++;
      if (l.diagnostic) {
        diagnostics++;
        const key = l.diagnostic.code;
        if (!categoryCounts[key]) {
          categoryCounts[key] = {
            title: l.diagnostic.title,
            count: 0,
            severity: l.diagnostic.severity === 'error' ? 'error' : 'warning',
          };
        }
        categoryCounts[key].count++;
      }
    });

    return {
      errors,
      warnings,
      diagnostics,
      problems: errors + warnings,
      categories: Object.entries(categoryCounts).map(([code, item]) => ({
        code,
        title: item.title,
        count: item.count,
        severity: item.severity,
      })),
    };
  }, [logs]);

  // Helper to extract terminal commands from remedy lines
  const parseRemedyLine = (remedy: string) => {
    // Check if the remedy references a command like `softwareupdate --install-rosetta` or `cupsctl ...`
    const commandRegex = /(?:running:\s*|run:\s*|command:\s*)([a-zA-Z0-9_\-\.\/]+(?:\s+[a-zA-Z0-9_\-\.\/=]+)+)/i;
    const match = remedy.match(commandRegex);
    if (match && match[1]) {
      const cmd = match[1].trim();
      const prefix = remedy.substring(0, match.index! + match[0].indexOf(cmd));
      return {
        prefix,
        command: cmd,
      };
    }
    return { prefix: remedy, command: null };
  };

  return (
    <div className="space-y-6">
      {/* Top Banner & Control Bar */}
      <div className="bg-white dark:bg-slate-800 rounded-2xl shadow-xs border border-slate-200 dark:border-slate-700 p-5">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-4 border-b border-slate-100 dark:border-slate-700">
          <div>
            <div className="flex items-center gap-2 flex-wrap">
              <h2 className="text-xl font-bold text-slate-900 dark:text-white">
                CUPS Server Logs &amp; Diagnostics
              </h2>
              {stats.errors > 0 && (
                <span className="px-2.5 py-0.5 text-xs font-semibold rounded-full bg-red-100 text-red-700 dark:bg-red-950/80 dark:text-red-300 border border-red-200 dark:border-red-800 flex items-center gap-1">
                  <XCircle className="h-3 w-3" />
                  {stats.errors} Error{stats.errors === 1 ? '' : 's'}
                </span>
              )}
              {stats.warnings > 0 && (
                <span className="px-2.5 py-0.5 text-xs font-semibold rounded-full bg-amber-100 text-amber-700 dark:bg-amber-950/80 dark:text-amber-300 border border-amber-200 dark:border-amber-800 flex items-center gap-1">
                  <AlertTriangle className="h-3 w-3" />
                  {stats.warnings} Warning{stats.warnings === 1 ? '' : 's'}
                </span>
              )}
              {stats.problems === 0 && !loading && (
                <span className="px-2.5 py-0.5 text-xs font-semibold rounded-full bg-emerald-100 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800 flex items-center gap-1">
                  <CheckCircle2 className="h-3 w-3" />
                  All Systems Healthy
                </span>
              )}
            </div>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
              Live streaming from <code className="text-xs bg-slate-100 dark:bg-slate-700 px-1.5 py-0.5 rounded text-slate-700 dark:text-slate-300">/var/log/cups/error_log</code> with automated root-cause analysis and actionable remedies.
            </p>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={() => fetchLogs()}
              disabled={loading}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg text-slate-700 dark:text-slate-200 bg-slate-100 dark:bg-slate-700 hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors disabled:opacity-50 cursor-pointer"
              title="Refresh logs"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin text-blue-600' : ''}`} />
              <span>Refresh</span>
            </button>

            <button
              onClick={handleCopy}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg text-slate-700 dark:text-slate-200 bg-slate-100 dark:bg-slate-700 hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors cursor-pointer"
              title="Copy visible logs to clipboard"
            >
              {copied ? (
                <>
                  <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />
                  <span className="text-emerald-600 dark:text-emerald-400">Copied!</span>
                </>
              ) : (
                <>
                  <Copy className="h-3.5 w-3.5" />
                  <span>Copy</span>
                </>
              )}
            </button>

            <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300 select-none cursor-pointer">
              <input
                type="checkbox"
                checked={autoRefresh}
                onChange={(e) => setAutoRefresh(e.target.checked)}
                className="h-3.5 w-3.5 rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer"
              />
              <span>Live (5s)</span>
            </label>
          </div>
        </div>

        {/* Quick Diagnostic Issue Category Filter Chips */}
        {stats.categories.length > 0 && (
          <div className="pt-3 pb-1 border-b border-slate-100 dark:border-slate-700">
            <div className="flex items-center gap-2 mb-2">
              <Wrench className="h-3.5 w-3.5 text-slate-400" />
              <span className="text-xs font-semibold text-slate-600 dark:text-slate-300">
                Identified Root-Cause Diagnostics:
              </span>
            </div>
            <div className="flex flex-wrap gap-2">
              {stats.categories.map((cat) => {
                const isActive = searchQuery.toLowerCase() === cat.title.toLowerCase();
                const isError = cat.severity === 'error';
                return (
                  <button
                    key={cat.code}
                    onClick={() => {
                      if (isActive) {
                        setSearchQuery('');
                      } else {
                        setSearchQuery(cat.title);
                      }
                    }}
                    className={`inline-flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-lg transition-all cursor-pointer ${
                      isActive
                        ? isError
                          ? 'bg-red-600 text-white font-bold shadow-xs'
                          : 'bg-amber-600 text-white font-bold shadow-xs'
                        : isError
                        ? 'bg-red-50 hover:bg-red-100 text-red-700 dark:bg-red-950/60 dark:hover:bg-red-900/80 dark:text-red-300 border border-red-200 dark:border-red-800'
                        : 'bg-amber-50 hover:bg-amber-100 text-amber-700 dark:bg-amber-950/60 dark:hover:bg-amber-900/80 dark:text-amber-300 border border-amber-200 dark:border-amber-800'
                    }`}
                  >
                    {isError ? (
                      <XCircle className="h-3 w-3 shrink-0" />
                    ) : (
                      <AlertTriangle className="h-3 w-3 shrink-0" />
                    )}
                    <span>{cat.title}</span>
                    <span
                      className={`px-1.5 py-0.2 rounded-full text-xs font-bold ${
                        isActive
                          ? 'bg-white/20 text-white'
                          : isError
                          ? 'bg-red-200/80 dark:bg-red-900 text-red-800 dark:text-red-200'
                          : 'bg-amber-200/80 dark:bg-amber-900 text-amber-800 dark:text-amber-200'
                      }`}
                    >
                      {cat.count}
                    </span>
                  </button>
                );
              })}

              {searchQuery && (
                <button
                  type="button"
                  onClick={() => setSearchQuery('')}
                  className="inline-flex items-center gap-1 px-2 py-1 text-xs text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-200 transition-colors cursor-pointer"
                >
                  <X className="h-3 w-3" aria-hidden="true" />
                  <span>Clear Filter</span>
                </button>
              )}
            </div>
          </div>
        )}

        {/* Filters and Search */}
        <div className="pt-4 flex flex-col md:flex-row gap-3 items-center justify-between">
          {/* Level Filter Tabs */}
          <div
            role="tablist"
            aria-label="Log severity filter"
            className="flex items-center gap-1 bg-slate-100 dark:bg-slate-900/80 border border-slate-200 dark:border-slate-700 p-1 rounded-lg w-full md:w-auto flex-wrap"
          >
            <button
              type="button"
              role="tab"
              aria-selected={levelFilter === 'all'}
              onClick={() => setLevelFilter('all')}
              className={`px-3 py-1 text-xs font-semibold rounded-md transition-all cursor-pointer ${
                levelFilter === 'all'
                  ? 'bg-white dark:bg-slate-800 text-slate-900 dark:text-white font-bold shadow-xs border border-slate-200 dark:border-slate-600'
                  : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
              }`}
            >
              All Levels
            </button>

            <button
              type="button"
              role="tab"
              aria-selected={levelFilter === 'problems'}
              onClick={() => setLevelFilter('problems')}
              className={`px-3 py-1 text-xs font-semibold rounded-md transition-all cursor-pointer flex items-center gap-1.5 ${
                levelFilter === 'problems'
                  ? 'bg-white dark:bg-slate-800 text-amber-600 dark:text-amber-400 font-bold shadow-xs border border-slate-200 dark:border-slate-600'
                  : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
              }`}
            >
              <AlertCircle className="h-3.5 w-3.5 text-amber-500" aria-hidden="true" />
              <span>Issues Only</span>
              {stats.problems > 0 && (
                <span className="px-1.5 py-0.2 rounded-full text-xs bg-amber-100 dark:bg-amber-950 text-amber-800 dark:text-amber-300 font-bold">
                  {stats.problems}
                </span>
              )}
            </button>

            <button
              type="button"
              role="tab"
              aria-selected={levelFilter === 'error'}
              onClick={() => setLevelFilter('error')}
              className={`px-3 py-1 text-xs font-semibold rounded-md transition-all cursor-pointer flex items-center gap-1.5 ${
                levelFilter === 'error'
                  ? 'bg-white dark:bg-slate-800 text-red-600 dark:text-red-400 font-bold shadow-xs border border-slate-200 dark:border-slate-600'
                  : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
              }`}
            >
              <XCircle className="h-3.5 w-3.5 text-red-500" aria-hidden="true" />
              <span>Errors</span>
              {stats.errors > 0 && (
                <span className="px-1.5 py-0.2 rounded-full text-xs bg-red-100 dark:bg-red-950 text-red-800 dark:text-red-300 font-bold">
                  {stats.errors}
                </span>
              )}
            </button>

            <button
              type="button"
              role="tab"
              aria-selected={levelFilter === 'warn'}
              onClick={() => setLevelFilter('warn')}
              className={`px-3 py-1 text-xs font-semibold rounded-md transition-all cursor-pointer flex items-center gap-1.5 ${
                levelFilter === 'warn'
                  ? 'bg-white dark:bg-slate-800 text-amber-600 dark:text-amber-400 font-bold shadow-xs border border-slate-200 dark:border-slate-600'
                  : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
              }`}
            >
              <AlertTriangle className="h-3.5 w-3.5 text-amber-500" aria-hidden="true" />
              <span>Warnings</span>
              {stats.warnings > 0 && (
                <span className="px-1.5 py-0.2 rounded-full text-xs bg-amber-100 dark:bg-amber-950 text-amber-800 dark:text-amber-300 font-bold">
                  {stats.warnings}
                </span>
              )}
            </button>

            <button
              type="button"
              role="tab"
              aria-selected={levelFilter === 'info'}
              onClick={() => setLevelFilter('info')}
              className={`px-3 py-1 text-xs font-semibold rounded-md transition-all cursor-pointer ${
                levelFilter === 'info'
                  ? 'bg-white dark:bg-slate-800 text-blue-600 dark:text-blue-400 font-bold shadow-xs border border-slate-200 dark:border-slate-600'
                  : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
              }`}
            >
              Info
            </button>
          </div>

          {/* Search Box */}
          <div className="relative w-full md:w-80">
            <label htmlFor="logs-search-input" className="sr-only">
              Search logs
            </label>
            <Search className="absolute left-3 top-2.5 h-3.5 w-3.5 text-slate-500 dark:text-slate-400" aria-hidden="true" />
            <input
              id="logs-search-input"
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search by printer, error, code, remedy..."
              aria-label="Search logs"
              className="w-full pl-9 pr-8 py-1.5 text-xs bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg text-slate-800 dark:text-slate-100 placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => setSearchQuery('')}
                aria-label="Clear search query"
                className="absolute right-2.5 top-2.5 text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 cursor-pointer"
              >
                <X className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
          </div>

          {/* Line Limit */}
          <div className="flex items-center gap-2 text-xs text-slate-500 dark:text-slate-400 self-end md:self-auto">
            <label htmlFor="logs-line-count">Show:</label>
            <select
              id="logs-line-count"
              value={lineCount}
              onChange={(e) => setLineCount(Number(e.target.value))}
              className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded px-2 py-1 text-xs text-slate-700 dark:text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500 cursor-pointer"
            >
              <option value={50}>50 lines</option>
              <option value={100}>100 lines</option>
              <option value={250}>250 lines</option>
              <option value={500}>500 lines</option>
            </select>
          </div>
        </div>
      </div>

      {/* Error Banner */}
      {error && (
        <div
          role="alert"
          className="p-4 bg-red-50 dark:bg-red-950/50 border border-red-200 dark:border-red-900 rounded-xl text-red-700 dark:text-red-300 flex items-center gap-3"
        >
          <XCircle className="h-5 w-5 shrink-0" aria-hidden="true" />
          <span className="text-xs">{error}</span>
        </div>
      )}

      {/* Unified Log Table */}
      <div className="bg-white dark:bg-slate-800 rounded-md border border-slate-200 dark:border-slate-700 overflow-hidden">
        <div className="hidden md:grid md:grid-cols-[170px_84px_1fr_auto] gap-3 py-2.5 px-4 bg-slate-50 dark:bg-slate-900/60 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider border-b border-slate-200 dark:border-slate-700">
          <span>Timestamp</span>
          <span>Level</span>
          <span>Event &amp; Queue</span>
          <span className="text-right">Diagnostics</span>
        </div>

        <div className="divide-y divide-slate-200 dark:divide-slate-700">
          {filteredLogs.length === 0 ? (
            <div className="p-12 text-center text-slate-500 dark:text-slate-400">
              {levelFilter === 'problems' || levelFilter === 'error' || levelFilter === 'warn' ? (
                <>
                  <CheckCircle2 className="h-10 w-10 mx-auto text-emerald-500 mb-2" aria-hidden="true" />
                  <p className="font-semibold text-sm text-slate-800 dark:text-slate-200">
                    No {levelFilter === 'problems' ? 'Errors or Warnings' : levelFilter === 'error' ? 'Errors' : 'Warnings'} Detected
                  </p>
                  <p className="text-xs mt-1 text-slate-500 dark:text-slate-400">
                    CUPS print queues and daemon background services are operating cleanly.
                  </p>
                </>
              ) : (
                <>
                  <Info className="h-8 w-8 mx-auto text-slate-400 mb-2" aria-hidden="true" />
                  <p className="font-medium text-sm text-slate-900 dark:text-white">No log entries matched your filter</p>
                  <p className="text-xs mt-1">Try changing your log level or clearing the search query.</p>
                </>
              )}
            </div>
          ) : (
            filteredLogs.map((entry, idx) => {
              const hasDiag = !!entry.diagnostic;
              const isExpanded = expandedDiagnostics[idx] ?? true;
              const isErrorDiag = entry.diagnostic?.severity === 'error';

              let badgeColor = 'bg-slate-100 text-slate-700 dark:bg-slate-700 dark:text-slate-300';
              let icon = <Info className="h-3.5 w-3.5 text-blue-500 shrink-0" />;
              if (entry.level === 'error') {
                badgeColor = 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300';
                icon = <XCircle className="h-3.5 w-3.5 text-red-500 shrink-0" />;
              } else if (entry.level === 'warn' || entry.level === 'warning') {
                badgeColor = 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300';
                icon = <AlertTriangle className="h-3.5 w-3.5 text-amber-500 shrink-0" />;
              } else if (entry.level === 'info') {
                badgeColor = 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300';
                icon = <Info className="h-3.5 w-3.5 text-blue-500 shrink-0" />;
              } else if (entry.level === 'debug') {
                badgeColor = 'bg-slate-100 text-slate-600 dark:bg-slate-700/70 dark:text-slate-400';
                icon = <Info className="h-3.5 w-3.5 text-slate-400 shrink-0" />;
              }

              return (
                <div
                  key={idx}
                  className={`hover:bg-slate-50/70 dark:hover:bg-slate-700/25 transition-colors ${
                    hasDiag
                      ? isErrorDiag
                        ? 'border-l-2 border-l-red-500'
                        : 'border-l-2 border-l-amber-500'
                      : ''
                  }`}
                >
                  {/* Standard Log Line Row */}
                  <div className="py-3 px-4 flex flex-col md:grid md:grid-cols-[170px_84px_1fr_auto] md:items-start gap-2.5 md:gap-3 text-xs font-mono">
                    <div className="flex items-center gap-2 text-slate-500 dark:text-slate-400 shrink-0 select-none">
                      <span className="shrink-0" aria-hidden="true">{icon}</span>
                      <span className="truncate">{entry.raw_date}</span>
                    </div>

                    <div>
                      <span
                        className={`inline-block text-xs px-2 py-0.5 rounded-sm font-bold uppercase tracking-wider ${badgeColor}`}
                      >
                        {entry.level}
                      </span>
                    </div>

                    <div className="flex items-start gap-2 flex-wrap min-w-0">
                      {entry.printer && (
                        <button
                          type="button"
                          onClick={() => setSearchQuery(entry.printer || '')}
                          title={`Filter logs for printer ${entry.printer}`}
                          className="text-xs px-2 py-0.5 rounded font-medium bg-blue-50 hover:bg-blue-100 text-blue-700 dark:bg-blue-950/70 dark:hover:bg-blue-900/80 dark:text-blue-300 border border-blue-200 dark:border-blue-800 shrink-0 cursor-pointer transition-colors"
                        >
                          {entry.printer}
                        </button>
                      )}

                      {entry.job_id && (
                        <button
                          type="button"
                          onClick={() => setSearchQuery(`Job ${entry.job_id}`)}
                          title={`Filter logs for Job #${entry.job_id}`}
                          className="text-xs px-2 py-0.5 rounded font-medium bg-indigo-50 hover:bg-indigo-100 text-indigo-700 dark:bg-indigo-950/70 dark:hover:bg-indigo-900/80 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800 shrink-0 cursor-pointer transition-colors"
                        >
                          Job #{entry.job_id}
                        </button>
                      )}

                      <span className="text-slate-800 dark:text-slate-200 break-words flex-1 font-mono text-xs leading-relaxed">
                        {entry.message}
                      </span>
                    </div>

                    <div className="flex justify-end">
                      {hasDiag && (
                        <button
                          type="button"
                          aria-expanded={isExpanded}
                          onClick={() => toggleDiagnostic(idx)}
                          className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-semibold rounded-md transition-colors cursor-pointer shrink-0 ${
                            isErrorDiag
                              ? 'bg-red-50 hover:bg-red-100 text-red-700 dark:bg-red-950/60 dark:hover:bg-red-900/80 dark:text-red-300 border border-red-200 dark:border-red-800'
                              : 'bg-amber-50 hover:bg-amber-100 text-amber-700 dark:bg-amber-950/60 dark:hover:bg-amber-900/80 dark:text-amber-300 border border-amber-200 dark:border-amber-800'
                          }`}
                        >
                          <Wrench className="h-3 w-3" aria-hidden="true" />
                          <span>{entry.diagnostic?.title}</span>
                          {isExpanded ? (
                            <ChevronUp className="h-3 w-3 ml-0.5" aria-hidden="true" />
                          ) : (
                            <ChevronDown className="h-3 w-3 ml-0.5" aria-hidden="true" />
                          )}
                        </button>
                      )}
                    </div>
                  </div>

                  {/* Expandable Diagnostic Remediation Box */}
                  {hasDiag && entry.diagnostic && isExpanded && (
                    <div
                      className={`mx-4 mb-3.5 p-3.5 rounded-md border ${
                        isErrorDiag
                          ? 'bg-red-50/50 dark:bg-red-950/20 border-red-200 dark:border-red-900/50'
                          : 'bg-amber-50/60 dark:bg-amber-950/20 border-amber-200 dark:border-amber-900/50'
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2 mb-2 flex-wrap">
                        <div className="flex items-center gap-2">
                          {isErrorDiag ? (
                            <XCircle className="h-4 w-4 text-red-600 dark:text-red-400 shrink-0" aria-hidden="true" />
                          ) : (
                            <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 shrink-0" aria-hidden="true" />
                          )}
                          <h4 className="text-xs font-bold text-slate-900 dark:text-white uppercase tracking-wider">
                            Diagnostic Assessment: {entry.diagnostic.title}
                          </h4>
                        </div>

                        <div className="flex items-center gap-1.5">
                          <span className="text-xs font-mono px-2 py-0.5 rounded bg-slate-200/80 dark:bg-slate-700 text-slate-700 dark:text-slate-300">
                            code: {entry.diagnostic.code}
                          </span>
                          <span
                            className={`text-xs font-bold uppercase tracking-wider px-2 py-0.5 rounded ${
                              isErrorDiag
                                ? 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300'
                                : 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300'
                            }`}
                          >
                            {entry.diagnostic.severity}
                          </span>
                        </div>
                      </div>

                      <p className="text-xs text-slate-700 dark:text-slate-300 mb-3 leading-relaxed">
                        {entry.diagnostic.explanation}
                      </p>

                      <div>
                        <span className="text-xs font-semibold text-slate-800 dark:text-slate-200 block mb-2">
                          Recommended Remediation Checklist:
                        </span>
                        <ul className="space-y-2">
                          {entry.diagnostic.remedies.map((remedy, rIdx) => {
                            const { prefix, command } = parseRemedyLine(remedy);
                            return (
                              <li
                                key={rIdx}
                                className="text-xs text-slate-600 dark:text-slate-300 flex items-start gap-2.5"
                              >
                                <span
                                  className={`inline-block h-1.5 w-1.5 rounded-full mt-1.5 shrink-0 ${
                                    isErrorDiag ? 'bg-red-500' : 'bg-amber-500'
                                  }`}
                                  aria-hidden="true"
                                />
                                <div className="flex-1">
                                  <span>{prefix}</span>
                                  {command && (
                                    <div className="mt-1 flex items-center gap-2">
                                      <code className="text-xs font-mono px-2 py-1 rounded bg-slate-100 dark:bg-slate-900 text-slate-800 dark:text-slate-200 border border-slate-200 dark:border-slate-700 inline-flex items-center gap-1.5">
                                        <Terminal className="h-3 w-3 text-slate-500 dark:text-slate-400" aria-hidden="true" />
                                        {command}
                                      </code>
                                      <button
                                        type="button"
                                        onClick={() => handleCopyCommand(command)}
                                        className="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded bg-slate-200 hover:bg-slate-300 dark:bg-slate-700 dark:hover:bg-slate-600 text-slate-700 dark:text-slate-200 transition-colors cursor-pointer"
                                        title="Copy command to clipboard"
                                      >
                                        {copiedCmd === command ? (
                                          <>
                                            <Check className="h-2.5 w-2.5 text-emerald-600 dark:text-emerald-400" aria-hidden="true" />
                                            <span>Copied</span>
                                          </>
                                        ) : (
                                          <>
                                            <Copy className="h-2.5 w-2.5" aria-hidden="true" />
                                            <span>Copy</span>
                                          </>
                                        )}
                                      </button>
                                    </div>
                                  )}
                                </div>
                              </li>
                            );
                          })}
                        </ul>
                      </div>
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
};
