import React, { useState } from 'react';
import {
  FileText,
  Clock,
  User,
  HardDrive,
  Search,
} from 'lucide-react';
import type { Job } from '../types/cups';
import { formatBytes } from '../utils/format';

interface JobQueueTableProps {
  activeJobs: Job[];
  completedJobs: Job[];
  onCancelJob: (id: string) => Promise<void>;
  onHoldJob: (id: string) => Promise<void>;
  onReleaseJob: (id: string) => Promise<void>;
  onRestartJob: (id: string) => Promise<void>;
}

export const JobQueueTable: React.FC<JobQueueTableProps> = ({
  activeJobs,
  completedJobs,
  onCancelJob,
  onHoldJob,
  onReleaseJob,
  onRestartJob,
}) => {
  const [tab, setTab] = useState<'active' | 'completed'>('active');
  const [search, setSearch] = useState('');
  const [busyJobId, setBusyJobId] = useState<string | null>(null);

  const currentJobs = tab === 'active' ? activeJobs : completedJobs;

  const filteredJobs = currentJobs.filter((job) => {
    const q = search.toLowerCase();
    return (
      job.title.toLowerCase().includes(q) ||
      job.user.toLowerCase().includes(q) ||
      job.printer.toLowerCase().includes(q) ||
      job.id.includes(q)
    );
  });

  const handleCancel = async (id: string) => {
    setBusyJobId(id);
    try {
      await onCancelJob(id);
    } finally {
      setBusyJobId(null);
    }
  };

  const handleHold = async (id: string) => {
    setBusyJobId(id);
    try {
      await onHoldJob(id);
    } finally {
      setBusyJobId(null);
    }
  };

  const handleRelease = async (id: string) => {
    setBusyJobId(id);
    try {
      await onReleaseJob(id);
    } finally {
      setBusyJobId(null);
    }
  };

  const handleRestart = async (id: string) => {
    setBusyJobId(id);
    try {
      await onRestartJob(id);
    } finally {
      setBusyJobId(null);
    }
  };

  // Status is a dot and a word. Only a stuck job is coloured as a problem.
  const getStatusBadge = (job: Job) => {
    const row = 'inline-flex items-center gap-2 text-sm text-slate-900 dark:text-white whitespace-nowrap';
    const dot = 'w-2 h-2 rounded-full shrink-0';
    if (job.is_stuck) {
      return (
        <span className={`${row} font-semibold !text-red-600 dark:!text-red-400`}>
          <span className={`${dot} bg-red-600 dark:bg-red-400`} />
          <span>Stuck</span>
        </span>
      );
    }

    const states: Record<string, { label: string; dot: string }> = {
      processing: { label: 'Printing', dot: 'bg-blue-600 dark:bg-blue-400 animate-pulse' },
      pending: { label: 'Queued', dot: 'bg-slate-400' },
      held: { label: 'Held', dot: 'bg-amber-500 dark:bg-amber-400' },
      completed: { label: 'Completed', dot: 'bg-blue-600 dark:bg-blue-400' },
      cancelled: { label: 'Cancelled', dot: 'bg-slate-400' },
    };
    const state = states[job.state] || { label: job.state, dot: 'bg-slate-400' };
    return (
      <span className={row}>
        <span className={`${dot} ${state.dot}`} />
        <span>{state.label}</span>
      </span>
    );
  };

  return (
    <div className="bg-white dark:bg-slate-800/90 rounded-2xl border border-slate-200 dark:border-slate-700 shadow-sm overflow-hidden">
      {/* Header with Tabs & Search */}
      <div className="p-4 sm:p-5 border-b border-slate-100 dark:border-slate-700 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div
          role="tablist"
          aria-label="Job queue filter"
          className="flex items-center space-x-2 bg-slate-100 dark:bg-slate-900/80 border border-slate-200 dark:border-slate-700 p-1 rounded-xl"
        >
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'active'}
            onClick={() => setTab('active')}
            className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition cursor-pointer ${
              tab === 'active'
                ? 'bg-white dark:bg-slate-800 text-slate-900 dark:text-white font-bold shadow-xs border border-slate-200 dark:border-slate-600'
                : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
            }`}
          >
            Active Queues ({activeJobs.length})
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'completed'}
            onClick={() => setTab('completed')}
            className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition cursor-pointer ${
              tab === 'completed'
                ? 'bg-white dark:bg-slate-800 text-slate-900 dark:text-white font-bold shadow-xs border border-slate-200 dark:border-slate-600'
                : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
            }`}
          >
            Completed History ({completedJobs.length})
          </button>
        </div>

        {/* Search */}
        <div className="relative">
          <label htmlFor="job-search-input" className="sr-only">
            Search print jobs
          </label>
          <Search className="w-4 h-4 text-slate-500 dark:text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" aria-hidden="true" />
          <input
            id="job-search-input"
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search jobs, printers, users..."
            aria-label="Search print jobs"
            className="pl-9 pr-3 py-1.5 text-xs rounded-xl bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 w-full sm:w-64"
          />
        </div>
      </div>

      {/* Table Content */}
      <div className="overflow-x-auto">
        <table className="w-full text-left border-collapse text-xs">
          <thead>
            <tr className="border-b border-slate-100 dark:border-slate-700 bg-slate-50/50 dark:bg-slate-900/30 text-slate-500 dark:text-slate-400 uppercase tracking-wider text-xs">
              <th className="py-3 px-4 font-semibold">Job ID</th>
              <th className="py-3 px-4 font-semibold">Document Title</th>
              <th className="py-3 px-4 font-semibold">Target Printer</th>
              <th className="py-3 px-4 font-semibold">User</th>
              <th className="py-3 px-4 font-semibold">Size</th>
              <th className="py-3 px-4 font-semibold">Submitted</th>
              <th className="py-3 px-4 font-semibold">Status</th>
              <th className="py-3 px-4 font-semibold text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-700">
            {filteredJobs.length === 0 ? (
              <tr>
                <td
                  colSpan={8}
                  className="py-12 text-center text-slate-500 dark:text-slate-400"
                >
                  <FileText className="w-8 h-8 mx-auto mb-2 opacity-40" aria-hidden="true" />
                  <p className="font-medium text-sm">
                    {tab === 'active' ? 'No active print jobs' : 'No completed jobs recorded'}
                  </p>
                  <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                    {tab === 'active'
                      ? 'Queues are clear and ready to accept print requests.'
                      : 'Recently finished jobs will be logged here.'}
                  </p>
                </td>
              </tr>
            ) : (
              filteredJobs.map((job) => (
                <tr
                  key={`${job.printer}-${job.id}`}
                  className={`hover:bg-slate-50/80 dark:hover:bg-slate-700/30 transition ${
                    ''
                  }`}
                >
                  <td className="py-3 px-4 font-mono font-medium text-slate-900 dark:text-slate-200">
                    #{job.id}
                  </td>
                  <td className="py-3 px-4 font-medium text-slate-900 dark:text-white max-w-xs truncate">
                    {job.title}
                  </td>
                  <td className="py-3 px-4">
                    <div className="flex items-center space-x-1.5 text-slate-600 dark:text-slate-300">
                      <HardDrive className="w-3.5 h-3.5 opacity-60" />
                      <span>{job.printer}</span>
                    </div>
                  </td>
                  <td className="py-3 px-4">
                    <div className="flex items-center space-x-1.5 text-slate-600 dark:text-slate-300">
                      <User className="w-3.5 h-3.5 opacity-60" />
                      <span>{job.user}</span>
                    </div>
                  </td>
                  <td className="py-3 px-4 text-slate-500 dark:text-slate-400 font-mono">
                    {formatBytes(job.size_bytes)}
                  </td>
                  <td className="py-3 px-4 text-slate-500 dark:text-slate-400 whitespace-nowrap">
                    <div className="flex items-center space-x-1.5">
                      <Clock className="w-3.5 h-3.5 opacity-60" />
                      <span>{job.raw_date || 'Recently'}</span>
                    </div>
                  </td>
                  <td className="py-3 px-4">{getStatusBadge(job)}</td>
                  <td className="py-3 px-4 text-right">
                    <div className="flex items-center justify-end space-x-1.5">
                      {tab === 'active' ? (
                        <>
                          {job.state === 'held' ? (
                            <button
                              onClick={() => handleRelease(job.id)}
                              disabled={busyJobId === job.id}
                              className="inline-flex items-center justify-center h-8 px-3 rounded-md text-[13px] font-semibold text-blue-700 dark:text-blue-400 hover:bg-slate-100 dark:hover:bg-slate-700/60 transition-colors cursor-pointer disabled:opacity-50"
                              title="Resume printing this held job"
                            >
                              <span>{busyJobId === job.id ? 'Resuming...' : 'Release'}</span>
                            </button>
                          ) : (
                            <button
                              onClick={() => handleHold(job.id)}
                              disabled={busyJobId === job.id}
                              className="inline-flex items-center justify-center h-8 px-3 rounded-md text-[13px] font-semibold text-blue-700 dark:text-blue-400 hover:bg-slate-100 dark:hover:bg-slate-700/60 transition-colors cursor-pointer disabled:opacity-50"
                              title="Hold this job indefinitely"
                            >
                              <span>{busyJobId === job.id ? 'Holding...' : 'Hold'}</span>
                            </button>
                          )}

                          <button
                            onClick={() => handleCancel(job.id)}
                            disabled={busyJobId === job.id}
                            className="inline-flex items-center justify-center h-8 px-3 rounded-md text-[13px] font-semibold text-slate-600 dark:text-slate-300 hover:text-red-600 dark:hover:text-red-400 hover:bg-slate-100 dark:hover:bg-slate-700/60 transition-colors cursor-pointer disabled:opacity-50"
                            title="Cancel this print job"
                          >
                            <span>{busyJobId === job.id ? 'Cancelling...' : 'Cancel'}</span>
                          </button>
                        </>
                      ) : (
                        <button
                          onClick={() => handleRestart(job.id)}
                          disabled={busyJobId === job.id}
                          className="inline-flex items-center justify-center h-8 px-3 rounded-md text-[13px] font-semibold text-blue-700 dark:text-blue-400 hover:bg-slate-100 dark:hover:bg-slate-700/60 transition-colors cursor-pointer disabled:opacity-50"
                          title="Reprint / Restart this job"
                        >
                          <span>{busyJobId === job.id ? 'Restarting...' : 'Reprint'}</span>
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};
