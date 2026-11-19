import React from 'react';
import type { Job } from '../types/cups';

interface StuckJobsBannerProps {
  stuckJobs: Job[];
  onCancelJob: (id: string) => Promise<void>;
}

export const StuckJobsBanner: React.FC<StuckJobsBannerProps> = ({ stuckJobs, onCancelJob }) => {
  if (stuckJobs.length === 0) return null;

  return (
    <div
      role="alert"
      className="mb-6 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 border-l-4 border-l-red-600 dark:border-l-red-400 rounded-md px-4 py-3"
    >
      <p className="text-sm text-slate-600 dark:text-slate-300">
        <strong className="font-semibold text-slate-900 dark:text-white">
          {stuckJobs.length === 1 ? '1 print job looks stuck.' : `${stuckJobs.length} print jobs look stuck.`}
        </strong>{' '}
        {stuckJobs.length === 1 ? 'It has' : 'They have'} been pending or processing for an unusually long time and may
        be blocking the queue.
      </p>
      <ul className="mt-2 divide-y divide-slate-200 dark:divide-slate-700">
        {stuckJobs.map((job) => (
          <li key={job.id} className="flex items-center gap-3 py-1.5 text-sm">
            <span className="font-mono text-xs text-slate-500 dark:text-slate-400 shrink-0">#{job.id}</span>
            <span className="font-semibold text-slate-900 dark:text-white truncate min-w-0" title={job.title}>
              {job.title}
            </span>
            <span className="hidden sm:inline text-slate-500 dark:text-slate-400 truncate min-w-0">
              on {job.printer}
            </span>
            <button
              type="button"
              onClick={() => onCancelJob(job.id)}
              className="ml-auto shrink-0 inline-flex items-center h-8 px-3 rounded-md text-sm font-semibold border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-slate-900 dark:text-white hover:border-red-600 hover:text-red-600 dark:hover:border-red-400 dark:hover:text-red-400 transition-colors cursor-pointer"
            >
              Cancel job
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
};
