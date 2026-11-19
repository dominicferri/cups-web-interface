import React, { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Info,
  Loader2,
  Pencil,
  RotateCcw,
  Search,
} from 'lucide-react';
import type {
  AccessoryOption,
  AccessoryReport,
  AccessorySource,
  AccessorySuggestions,
} from '../types/cups';
import { api } from '../api/client';
import { choiceLabel, isFitted } from '../utils/accessories';

// Text from the feature brief; shown before every save.
const WARNING_TEXT =
  "These settings can't be checked against the printer. If you enable an accessory the printer doesn't have, users can choose it, but jobs may print without it or be held by the printer. Confirm the target printer has this hardware.";

// Badges are shown only for values that are not the driver default: those are
// the exceptions an admin needs to notice.
const SOURCE_BADGES: Partial<Record<AccessorySource, { label: string; title: string; className: string }>> = {
  manual: {
    label: 'Set manually',
    title: 'Set by an administrator in this app.',
    className: 'bg-violet-100 text-violet-800 border-violet-300 dark:bg-violet-950 dark:text-violet-200 dark:border-violet-800',
  },
  detected: {
    label: 'Detected',
    title:
      "Differs from the driver's default and wasn't set in this app: found by the driver's setup when the queue was added, or changed outside this app.",
    className: 'bg-sky-100 text-sky-800 border-sky-300 dark:bg-sky-950 dark:text-sky-200 dark:border-sky-800',
  },
};

const UNAVAILABLE_TEXT: Record<string, string> = {
  driverless:
    'This is a driverless queue (AirPrint / IPP Everywhere). It has no installable options: the printer reports its own features.',
  no_installable_options: "This queue's driver doesn't declare any installable options (trays, finishers).",
  no_ppd: 'This queue has no PPD, so it has no installable options.',
};

function needsAttention(opt: AccessoryOption): boolean {
  return opt.source === 'manual' || opt.source === 'detected' || opt.override_replaced;
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

const SourceBadge: React.FC<{ source: AccessorySource }> = ({ source }) => {
  const s = SOURCE_BADGES[source];
  if (!s) return null;
  return (
    <span title={s.title} className={`px-2 py-0.5 rounded-full text-xs font-semibold border whitespace-nowrap ${s.className}`}>
      {s.label}
    </span>
  );
};

interface AccessoriesPanelProps {
  printerName: string;
  onChanged?: () => void;
  // Called with every report loaded or saved, so the parent can update the tab count.
  onReport?: (report: AccessoryReport) => void;
  // A report the parent already loaded; when given, the panel doesn't fetch it again.
  initialReport?: AccessoryReport | null;
}

function draftFrom(rep: AccessoryReport | null | undefined): Record<string, string> {
  const values: Record<string, string> = {};
  rep?.options.forEach((o) => {
    values[o.key] = o.value;
  });
  return values;
}

export const AccessoriesPanel: React.FC<AccessoriesPanelProps> = ({ printerName, onChanged, onReport, initialReport }) => {
  const [report, setReport] = useState<AccessoryReport | null>(initialReport ?? null);
  const [loading, setLoading] = useState(!initialReport);
  // Only the report given at mount counts; later ones come back from onReport.
  const [hadInitialReport] = useState(Boolean(initialReport));
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [draft, setDraft] = useState<Record<string, string>>(() => draftFrom(initialReport));
  const [editing, setEditing] = useState(false);
  const [showAll, setShowAll] = useState(false);
  const [confirming, setConfirming] = useState<'save' | 'reset' | null>(null);
  const [hardwareConfirmed, setHardwareConfirmed] = useState(false);
  const [saving, setSaving] = useState(false);
  const [suggestions, setSuggestions] = useState<AccessorySuggestions | null>(null);
  const [checking, setChecking] = useState(false);
  const warningId = useId();
  const topRef = useRef<HTMLDivElement>(null);
  const warningRef = useRef<HTMLDivElement>(null);

  const applyReport = useCallback((rep: AccessoryReport) => {
    setReport(rep);
    onReport?.(rep);
    setDraft(draftFrom(rep));
  }, [onReport]);

  // The parent keys this component by queue name, so state starts fresh for each queue.
  useEffect(() => {
    if (hadInitialReport) return;
    let cancelled = false;
    api
      .getAccessories(printerName)
      .then((rep) => {
        if (!cancelled) applyReport(rep);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load accessories');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [printerName, applyReport, hadInitialReport]);

  useEffect(() => {
    if (confirming && warningRef.current) {
      warningRef.current.focus({ preventScroll: true });
    }
  }, [confirming]);

  // Results are shown at the top of the panel; bring them into view.
  useEffect(() => {
    if ((success || error) && topRef.current) {
      topRef.current.scrollIntoView?.({ block: 'start', behavior: 'smooth' });
    }
  }, [success, error]);

  const changes = useMemo(() => {
    if (!report) return [];
    return report.options.filter((o) => draft[o.key] !== undefined && draft[o.key] !== o.value);
  }, [report, draft]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-10 text-xs text-slate-500 dark:text-slate-400 space-x-2">
        <Loader2 className="w-4 h-4 animate-spin text-blue-600" aria-hidden="true" />
        <span>Reading installable options from the queue's PPD...</span>
      </div>
    );
  }

  if (!report) {
    return (
      <div role="alert" className="p-3 rounded-xl bg-rose-50 dark:bg-rose-950/50 border border-rose-200 dark:border-rose-800 text-rose-700 dark:text-rose-300 text-xs">
        {error || 'Failed to load accessories'}
      </div>
    );
  }

  if (!report.available) {
    return (
      <div className="text-center py-10 text-slate-500 dark:text-slate-400 text-xs">
        {UNAVAILABLE_TEXT[report.unavailable_reason || ''] ||
          "No installable hardware accessories (trays, finishers) declared in this printer's PPD."}
      </div>
    );
  }

  const fitted = report.options.filter((o) => isFitted(o));
  // Listed first: fitted accessories and anything that isn't the driver default.
  const primary = report.options.filter((o) => isFitted(o) || needsAttention(o));
  const rest = report.options.filter((o) => !primary.includes(o));
  const expanded = showAll || editing;
  const strongWarning = !report.direct_connection;
  const canConfirm = !saving && (!strongWarning || hardwareConfirmed);

  const resetDraft = () => {
    const values: Record<string, string> = {};
    report.options.forEach((o) => {
      values[o.key] = o.value;
    });
    setDraft(values);
  };

  const startEditing = () => {
    resetDraft();
    setEditing(true);
    setSuccess(null);
    setError(null);
  };

  const stopEditing = () => {
    resetDraft();
    setEditing(false);
    setConfirming(null);
    setSuggestions(null);
  };

  const startConfirm = (kind: 'save' | 'reset') => {
    setConfirming(kind);
    setHardwareConfirmed(false);
    setError(null);
    setSuccess(null);
  };

  const finish = (rep: AccessoryReport, message: string) => {
    applyReport(rep);
    setEditing(false);
    setConfirming(null);
    setSuggestions(null);
    setSuccess(message);
    onChanged?.();
  };

  const fail = (err: unknown, fallback: string) => {
    setError(err instanceof Error ? err.message : fallback);
    setConfirming(null);
    // Show what CUPS holds now, whatever happened.
    api.getAccessories(printerName).then(applyReport).catch(() => undefined);
  };

  const handleSave = async () => {
    const values: Record<string, string> = {};
    changes.forEach((o) => {
      values[o.key] = draft[o.key];
    });
    setSaving(true);
    setError(null);
    try {
      const rep = await api.setAccessories(printerName, values, true);
      const stored = Object.keys(values)
        .map((k) => {
          const o = rep.options.find((x) => x.key === k);
          return o ? `${o.label}: ${choiceLabel(o, o.value)}` : k;
        })
        .join(', ');
      finish(rep, `Saved. CUPS now stores ${stored}.`);
    } catch (err) {
      fail(err, 'Failed to save accessories');
    } finally {
      setSaving(false);
    }
  };

  const handleReset = async () => {
    setSaving(true);
    setError(null);
    try {
      finish(await api.resetAccessories(printerName), 'Manual settings removed. The queue has its detected values again.');
    } catch (err) {
      fail(err, 'Failed to reset accessories');
    } finally {
      setSaving(false);
    }
  };

  const handleCheckPrinter = async () => {
    setChecking(true);
    setError(null);
    try {
      setSuggestions(await api.suggestAccessories(printerName));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to query the printer');
    } finally {
      setChecking(false);
    }
  };

  const renderRow = (o: AccessoryOption) => {
    const id = `manual-acc-${o.key}`;
    const changed = editing && draft[o.key] !== o.value;
    return (
      <li key={o.key} className="px-3 py-2 text-xs flex items-start justify-between gap-3">
        <div className="min-w-0">
          {editing ? (
            <label htmlFor={id} title={o.key} className="font-semibold text-slate-800 dark:text-slate-200">
              {o.label}
            </label>
          ) : (
            <span title={o.key} className="font-semibold text-slate-800 dark:text-slate-200">
              {o.label}
            </span>
          )}
          {changed && <span className="ml-2 text-violet-700 dark:text-violet-300">(changed)</span>}
          {o.override && !o.override_replaced && (
            <p className="text-slate-500 dark:text-slate-400 mt-0.5">
              <span title="Recorded by this app on this Mac, for the account running it. A note, not an audit log.">
                Set by {o.override.set_by} on {formatDate(o.override.set_at)}
              </span>
              {o.override.previous_value && ` (was ${choiceLabel(o, o.override.previous_value)})`}
            </p>
          )}
          {o.override && o.override_replaced && (
            <p className="text-amber-700 dark:text-amber-300 mt-0.5">
              Set manually to {choiceLabel(o, o.override.value)} by {o.override.set_by}, but the queue now has{' '}
              {choiceLabel(o, o.value)}. It was changed outside this app.
            </p>
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {editing ? (
            <select
              id={id}
              value={draft[o.key] ?? o.value}
              disabled={saving || confirming !== null}
              onChange={(e) => setDraft((prev) => ({ ...prev, [o.key]: e.target.value }))}
              className="w-56 px-2 py-1 text-xs rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-slate-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              {o.choices.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.label}
                  {c.name === o.driver_default ? ' (driver default)' : ''}
                </option>
              ))}
            </select>
          ) : (
            <span className={isFitted(o) ? 'text-slate-800 dark:text-slate-200 font-medium' : 'text-slate-500 dark:text-slate-400'}>
              {choiceLabel(o, o.value)}
            </span>
          )}
          <SourceBadge source={o.source} />
        </div>
      </li>
    );
  };

  return (
    <div className="space-y-4">
      <div ref={topRef} className="space-y-2 scroll-mt-6">
        {error && (
          <div role="alert" className="p-3 rounded-xl bg-rose-50 dark:bg-rose-950/50 border border-rose-200 dark:border-rose-800 text-rose-700 dark:text-rose-300 text-xs flex items-start space-x-2">
            <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" aria-hidden="true" />
            <span>{error}</span>
          </div>
        )}
        {success && (
          <div role="status" aria-live="polite" className="p-3 rounded-xl bg-emerald-50 dark:bg-emerald-950/50 border border-emerald-200 dark:border-emerald-800 text-emerald-700 dark:text-emerald-300 text-xs flex items-start space-x-2">
            <CheckCircle2 className="w-4 h-4 shrink-0 mt-0.5" aria-hidden="true" />
            <span>{success}</span>
          </div>
        )}
      </div>

      {/* Summary */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-bold text-slate-900 dark:text-white">
            {editing ? 'Manual override' : `${fitted.length} of ${report.options.length} installed`}
            {editing && (
              <span className="ml-2 px-2 py-0.5 rounded-full text-xs font-semibold border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 align-middle">
                Advanced
              </span>
            )}
          </h3>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
            {editing
              ? `For when the driver couldn't detect the printer's hardware. Same as lpadmin -p ${printerName} -o Option=Value; only this queue changes.`
              : fitted.length > 0
                ? fitted.map((o) => o.label).join(', ')
                : 'No accessories are marked as installed.'}
          </p>
        </div>
        {report.is_admin && !editing && (
          <button
            type="button"
            onClick={startEditing}
            disabled={Boolean(report.history_error)}
            title={report.history_error ? 'Changes are disabled until the history file is fixed' : undefined}
            className="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            <Pencil className="w-3.5 h-3.5" aria-hidden="true" />
            <span>Edit accessories</span>
          </button>
        )}
      </div>

      {report.warning && (
        <div role="alert" className="p-3 rounded-xl border text-xs bg-amber-50 dark:bg-amber-950/40 border-amber-300 dark:border-amber-800 text-amber-900 dark:text-amber-200 flex items-start gap-2">
          <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" aria-hidden="true" />
          <span>{report.warning}</span>
        </div>
      )}
      {report.history_error && (
        <div role="alert" className="p-3 rounded-xl border text-xs bg-amber-50 dark:bg-amber-950/40 border-amber-300 dark:border-amber-800 text-amber-900 dark:text-amber-200 flex items-start gap-2">
          <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" aria-hidden="true" />
          <span>
            The values below come from CUPS, but who changed them can't be shown: {report.history_error}
          </span>
        </div>
      )}
      {Boolean(report.ignored_notes) && (
        <p className="text-xs text-slate-500 dark:text-slate-400 flex items-start gap-1.5">
          <Info className="w-3.5 h-3.5 shrink-0 mt-0.5" aria-hidden="true" />
          <span>
            Manual settings recorded for an earlier queue with this name are ignored. They are removed with the next change.
          </span>
        </p>
      )}

      {!report.driver_defaults_known && (
        <p className="text-xs text-slate-500 dark:text-slate-400 flex items-start gap-1.5">
          <Info className="w-3.5 h-3.5 shrink-0 mt-0.5" aria-hidden="true" />
          <span>The driver's original PPD couldn't be read, so values can't be compared with the driver defaults.</span>
        </p>
      )}

      {/* Edit-mode context: connection warning and suggestions */}
      {editing && (
        <div className="space-y-3">
          {strongWarning && (
            <div className="p-3 rounded-xl border text-xs bg-amber-50 dark:bg-amber-950/40 border-amber-300 dark:border-amber-800 text-amber-900 dark:text-amber-200">
              <p className="font-semibold">This queue doesn't point straight at the printer.</p>
              <p className="mt-0.5">{report.connection_note}</p>
              <p className="mt-0.5 font-mono break-all opacity-80">{report.device_uri}</p>
            </div>
          )}
          <div className="space-y-2">
            <button
              type="button"
              onClick={handleCheckPrinter}
              disabled={checking || saving}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700 disabled:opacity-50 cursor-pointer"
            >
              {checking ? <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" /> : <Search className="w-3.5 h-3.5" aria-hidden="true" />}
              <span>Ask the device for suggestions (IPP)</span>
            </button>
            {suggestions && (
              <div className="p-3 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs space-y-2" role="status">
                {suggestions.suggestions.length > 0 && (
                  <ul className="space-y-1.5">
                    {suggestions.suggestions.map((s) => {
                      const opt = report.options.find((o) => o.key === s.key);
                      const applied = draft[s.key] === s.value;
                      return (
                        <li key={s.key} className="flex items-center justify-between gap-3">
                          <div className="min-w-0">
                            <span className="font-semibold text-slate-800 dark:text-slate-200">
                              {s.label}: {opt ? choiceLabel(opt, s.value) : s.value}
                            </span>
                            <p className="text-slate-500 dark:text-slate-400">{s.evidence}</p>
                          </div>
                          <button
                            type="button"
                            disabled={applied || saving || confirming !== null}
                            onClick={() => setDraft((prev) => ({ ...prev, [s.key]: s.value }))}
                            className="shrink-0 px-2 py-1 rounded-lg text-xs font-semibold border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700 disabled:opacity-50 cursor-pointer"
                          >
                            {applied ? 'In form' : 'Use'}
                          </button>
                        </li>
                      );
                    })}
                  </ul>
                )}
                {suggestions.note && <p className="text-slate-500 dark:text-slate-400">{suggestions.note}</p>}
                {suggestions.suggestions.length > 0 && (
                  <p className="text-slate-500 dark:text-slate-400">Suggestions only fill in the form. Nothing is saved until you save.</p>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* The one list: read-only, or editable in Edit mode */}
      <div className="rounded-xl border border-slate-200 dark:border-slate-800">
        {primary.length > 0 && (
          <ul className="divide-y divide-slate-100 dark:divide-slate-800">{primary.map(renderRow)}</ul>
        )}
        {rest.length > 0 && (
          <>
            <button
              type="button"
              onClick={() => setShowAll((v) => !v)}
              disabled={editing}
              aria-expanded={expanded}
              className={`w-full flex items-center gap-1.5 px-3 py-2 text-xs font-semibold text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-800/50 disabled:cursor-default cursor-pointer ${
                primary.length > 0 ? 'border-t border-slate-200 dark:border-slate-800' : ''
              } ${expanded ? 'border-b border-slate-200 dark:border-slate-800' : 'rounded-b-xl'} ${primary.length === 0 ? 'rounded-t-xl' : ''}`}
            >
              {expanded ? <ChevronDown className="w-3.5 h-3.5" aria-hidden="true" /> : <ChevronRight className="w-3.5 h-3.5" aria-hidden="true" />}
              <span>
                {expanded ? '' : 'Show '}
                {rest.length} more not installed
              </span>
            </button>
            {expanded && <ul className="divide-y divide-slate-100 dark:divide-slate-800">{rest.map(renderRow)}</ul>}
          </>
        )}
      </div>

      {/* Sticky action bar for Edit mode */}
      {editing && (
        <div className="sticky bottom-0 -mx-6 -mb-6 px-6 py-3 bg-white/95 dark:bg-slate-900/95 backdrop-blur-md border-t border-slate-200 dark:border-slate-800 space-y-3">
          {confirming && (
            <div
              ref={warningRef}
              tabIndex={-1}
              role="alertdialog"
              aria-labelledby={`${warningId}-title`}
              aria-describedby={`${warningId}-text`}
              className={`p-3 rounded-xl border space-y-2 text-xs max-h-[50vh] overflow-y-auto focus:outline-none ${
                strongWarning
                  ? 'bg-rose-50 dark:bg-rose-950/40 border-rose-300 dark:border-rose-800 text-rose-900 dark:text-rose-100'
                  : 'bg-amber-50 dark:bg-amber-950/40 border-amber-300 dark:border-amber-800 text-amber-900 dark:text-amber-100'
              }`}
            >
              <div className="flex items-center gap-2 font-bold" id={`${warningId}-title`}>
                <AlertTriangle className="w-4 h-4 shrink-0" aria-hidden="true" />
                <span>
                  {confirming === 'save'
                    ? `Save ${changes.length} manual ${changes.length === 1 ? 'setting' : 'settings'}?`
                    : 'Reset to detected values?'}
                </span>
              </div>
              <div id={`${warningId}-text`} className="space-y-2">
                {confirming === 'save' ? (
                  <>
                    <ul className="list-disc pl-5">
                      {changes.map((o) => (
                        <li key={o.key}>
                          {o.label}: {choiceLabel(o, o.value)} → {choiceLabel(o, draft[o.key])}
                        </li>
                      ))}
                    </ul>
                    <p>{WARNING_TEXT}</p>
                    {strongWarning && (
                      <p className="font-semibold">
                        This queue doesn't send jobs straight to the printer ({report.connection_note}). This Mac
                        can't see what hardware is fitted to the physical printer behind it, and the server may not
                        report it either. Check the printer itself before you save.
                      </p>
                    )}
                  </>
                ) : (
                  <p>
                    Each manually set option goes back to the value it had before it was first changed in this app,
                    and the manual settings are forgotten.
                  </p>
                )}
              </div>
              {confirming === 'save' && strongWarning && (
                <label className="flex items-start gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={hardwareConfirmed}
                    onChange={(e) => setHardwareConfirmed(e.target.checked)}
                    className="mt-0.5 w-4 h-4 rounded border-slate-300 cursor-pointer"
                  />
                  <span className="font-semibold">I've confirmed the target printer has this hardware.</span>
                </label>
              )}
            </div>
          )}

          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              {!confirming && (
                <button
                  type="button"
                  onClick={() => startConfirm('reset')}
                  disabled={!report.has_overrides || saving}
                  title={report.has_overrides ? undefined : 'No manual settings to reset'}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold text-slate-700 dark:text-slate-200 hover:bg-slate-200 dark:hover:bg-slate-700 disabled:opacity-50 cursor-pointer"
                >
                  <RotateCcw className="w-3.5 h-3.5" aria-hidden="true" />
                  <span>Reset to detected values</span>
                </button>
              )}
            </div>
            <div className="flex items-center gap-2">
              {!confirming && (
                <span className="text-xs text-slate-500 dark:text-slate-400">
                  {changes.length === 0 ? 'No changes' : `${changes.length} ${changes.length === 1 ? 'change' : 'changes'}`}
                </span>
              )}
              <button
                type="button"
                onClick={confirming ? () => setConfirming(null) : stopEditing}
                disabled={saving}
                className="px-3 py-1.5 rounded-lg text-xs font-semibold text-slate-700 dark:text-slate-200 hover:bg-slate-200 dark:hover:bg-slate-700 cursor-pointer"
              >
                {confirming ? 'Back' : 'Cancel'}
              </button>
              {confirming ? (
                <button
                  type="button"
                  onClick={confirming === 'save' ? handleSave : handleReset}
                  disabled={confirming === 'save' ? !canConfirm : saving}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 cursor-pointer"
                >
                  {saving && <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" />}
                  <span>{confirming === 'save' ? 'Save to CUPS' : 'Reset'}</span>
                </button>
              ) : (
                <button
                  type="button"
                  onClick={() => startConfirm('save')}
                  disabled={changes.length === 0 || saving}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 cursor-pointer"
                >
                  <span>Review and save…</span>
                </button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
