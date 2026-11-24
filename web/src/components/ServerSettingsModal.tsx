import React, { useState, useEffect, useId } from 'react';
import {
  X,
  Settings,
  ShieldCheck,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Share2,
  Globe,
  FileCode,
  Save,
  Power,
  Trash2,
  Radar,
} from 'lucide-react';
import type {
  ServerSettings,
  LaunchAgentStatus,
  AccessoryFeatureStatus,
  InventoryStatus,
  InventoryImportResult,
} from '../types/cups';
import { api } from '../api/client';
import { useModalDialog } from '../utils/useModalDialog';

interface ServerSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: () => void;
}

export const ServerSettingsModal: React.FC<ServerSettingsModalProps> = ({
  isOpen,
  onClose,
  onSuccess,
}) => {
  const [settings, setSettings] = useState<ServerSettings | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedSuccess, setSavedSuccess] = useState(false);
  const titleId = useId();
  const dialogRef = useModalDialog(isOpen, onClose);

  // macOS LaunchAgent state
  const [launchAgent, setLaunchAgent] = useState<LaunchAgentStatus | null>(null);
  const [launchAgentLoading, setLaunchAgentLoading] = useState(false);
  const [launchAgentActionLoading, setLaunchAgentActionLoading] = useState(false);
  const [launchAgentMsg, setLaunchAgentMsg] = useState<string | null>(null);

  // Optional SNMP hardware check (off by default)
  const [features, setFeatures] = useState<AccessoryFeatureStatus | null>(null);
  const [featuresSaving, setFeaturesSaving] = useState(false);
  const snmpToggleId = useId();
  const csvInputId = useId();
  const [inventory, setInventory] = useState<InventoryStatus | null>(null);
  const [importResult, setImportResult] = useState<InventoryImportResult | null>(null);
  const [inventoryError, setInventoryError] = useState<string | null>(null);
  const [inventoryBusy, setInventoryBusy] = useState(false);

  const fetchLaunchAgent = async () => {
    try {
      setLaunchAgentLoading(true);
      const status = await api.getLaunchAgentStatus();
      setLaunchAgent(status);
    } catch {
      // Ignore background error if unsupported
    } finally {
      setLaunchAgentLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      setError(null);
      setSavedSuccess(false);
      setLoading(true);
      setLaunchAgentMsg(null);
      api
        .getServerSettings()
        .then((s) => setSettings(s))
        .catch((err) => {
          const msg = err instanceof Error ? err.message : 'Failed to load CUPS server settings';
          setError(msg);
        })
        .finally(() => setLoading(false));

      fetchLaunchAgent();
      api
        .getAccessoryFeatures()
        .then(setFeatures)
        .catch(() => setFeatures(null));
      setImportResult(null);
      setInventoryError(null);
      api
        .getPrinterInventory()
        .then(setInventory)
        .catch(() => setInventory(null));
    }
  }, [isOpen]);

  const handleImportCsv = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = ''; // choosing the same file again re-imports it
    if (!file) return;
    setInventoryBusy(true);
    setInventoryError(null);
    setImportResult(null);
    try {
      const res = await api.importPrinterInventory(file);
      setImportResult(res);
      setInventory(res);
    } catch (err) {
      setInventoryError(err instanceof Error ? err.message : 'Failed to import the printer list');
    } finally {
      setInventoryBusy(false);
    }
  };

  const handleRemoveCsv = async () => {
    setInventoryBusy(true);
    setInventoryError(null);
    setImportResult(null);
    try {
      setInventory(await api.removePrinterInventory());
    } catch (err) {
      setInventoryError(err instanceof Error ? err.message : 'Failed to remove the printer list');
    } finally {
      setInventoryBusy(false);
    }
  };

  const handleToggleSnmp = async (enabled: boolean) => {
    setFeaturesSaving(true);
    setError(null);
    try {
      setFeatures(await api.setAccessoryFeatures({ snmp_enabled: enabled }));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save the SNMP setting');
    } finally {
      setFeaturesSaving(false);
    }
  };

  const handleManageLaunchAgent = async (action: 'install' | 'uninstall') => {
    setLaunchAgentActionLoading(true);
    setLaunchAgentMsg(null);
    try {
      const res = await api.manageLaunchAgent(action);
      setLaunchAgentMsg(res.message);
      await fetchLaunchAgent();
      setTimeout(() => setLaunchAgentMsg(null), 4000);
    } catch (err: any) {
      setError(err?.message || `Failed to ${action} LaunchAgent`);
    } finally {
      setLaunchAgentActionLoading(false);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!settings) return;

    setSaving(true);
    setError(null);
    setSavedSuccess(false);

    try {
      await api.setServerSettings(settings);
      setSavedSuccess(true);
      if (onSuccess) onSuccess();
      setTimeout(() => {
        setSavedSuccess(false);
      }, 3000);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to update server settings';
      setError(msg);
    } finally {
      setSaving(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 overflow-hidden bg-slate-900/60 backdrop-blur-xs flex items-center justify-center p-4 animate-in fade-in duration-200">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="relative w-full max-w-lg bg-white dark:bg-slate-900 rounded-3xl shadow-2xl border border-slate-200 dark:border-slate-800 overflow-hidden animate-in zoom-in-95 duration-200 focus:outline-none"
      >
        
        {/* Header */}
        <div className="px-6 py-5 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="p-2.5 bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 rounded-2xl">
              <Settings className="w-5 h-5" aria-hidden="true" />
            </div>
            <div>
              <h3 id={titleId} className="text-base font-bold text-slate-900 dark:text-white">
                CUPS Server Settings
              </h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                Daemon parameters managed via cupsctl
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
            className="p-1.5 rounded-xl text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
          >
            <X className="w-5 h-5" aria-hidden="true" />
          </button>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSave}>
          <div className="p-6 space-y-4">
            {error && (
              <div
                role="alert"
                className="p-3.5 rounded-xl bg-rose-50 dark:bg-rose-950/50 border border-rose-200 dark:border-rose-900/60 flex items-start space-x-2.5"
              >
                <AlertCircle className="w-4 h-4 text-rose-600 dark:text-rose-400 shrink-0 mt-0.5" aria-hidden="true" />
                <p className="text-xs text-rose-800 dark:text-rose-300">{error}</p>
              </div>
            )}

            {savedSuccess && (
              <div
                role="status"
                aria-live="polite"
                className="p-3.5 rounded-xl bg-emerald-50 dark:bg-emerald-950/50 border border-emerald-200 dark:border-emerald-900/60 flex items-center space-x-2.5"
              >
                <CheckCircle2 className="w-4 h-4 text-emerald-600 dark:text-emerald-400 shrink-0" aria-hidden="true" />
                <p className="text-xs text-emerald-800 dark:text-emerald-300 font-medium">
                  CUPS server configuration applied successfully.
                </p>
              </div>
            )}

            {loading || !settings ? (
              <div className="py-12 flex flex-col items-center justify-center space-y-3">
                <Loader2 className="w-7 h-7 animate-spin text-blue-600" aria-hidden="true" />
                <p className="text-xs text-slate-500 dark:text-slate-400">Querying CUPS daemon via cupsctl...</p>
              </div>
            ) : (
              <div className="space-y-4">
                {/* Setting 1: Share Printers */}
                <div className="flex items-start space-x-3 p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800">
                  <Share2 className="w-5 h-5 text-slate-500 dark:text-slate-400 shrink-0 mt-0.5" aria-hidden="true" />
                  <div className="flex-1">
                    <label
                      htmlFor="setting-share-printers"
                      className="text-xs font-semibold text-slate-900 dark:text-white block cursor-pointer"
                    >
                      Share connected printers on the local network
                    </label>
                    <p
                      id="setting-share-printers-desc"
                      className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed"
                    >
                      Enables cupsd to broadcast shared queues to other devices using Bonjour / mDNS.
                    </p>
                  </div>
                  <input
                    id="setting-share-printers"
                    aria-describedby="setting-share-printers-desc"
                    type="checkbox"
                    checked={settings.share_printers}
                    onChange={(e) =>
                      setSettings({ ...settings, share_printers: e.target.checked })
                    }
                    className="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 mt-1 cursor-pointer"
                  />
                </div>

                {/* Setting 2: Remote Any / Internet Access */}
                <div className="flex items-start space-x-3 p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800">
                  <Globe className="w-5 h-5 text-indigo-600 dark:text-indigo-400 shrink-0 mt-0.5" aria-hidden="true" />
                  <div className="flex-1">
                    <label
                      htmlFor="setting-remote-printers"
                      className="text-xs font-semibold text-slate-900 dark:text-white block cursor-pointer"
                    >
                      Allow printing from the Internet (Remote Any)
                    </label>
                    <p
                      id="setting-remote-printers-desc"
                      className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed"
                    >
                      Allows CUPS to accept connections and print jobs from any IP address beyond the local network.
                    </p>
                  </div>
                  <input
                    id="setting-remote-printers"
                    aria-describedby="setting-remote-printers-desc"
                    type="checkbox"
                    checked={settings.remote_printers}
                    onChange={(e) =>
                      setSettings({ ...settings, remote_printers: e.target.checked })
                    }
                    className="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 mt-1 cursor-pointer"
                  />
                </div>

                {/* Setting 3: Debug Logging */}
                <div className="flex items-start space-x-3 p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800">
                  <FileCode className="w-5 h-5 text-slate-500 dark:text-slate-400 shrink-0 mt-0.5" aria-hidden="true" />
                  <div className="flex-1">
                    <label
                      htmlFor="setting-debug-logging"
                      className="text-xs font-semibold text-slate-900 dark:text-white block cursor-pointer"
                    >
                      Save debugging information for troubleshooting (Debug Logging)
                    </label>
                    <p
                      id="setting-debug-logging-desc"
                      className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed"
                    >
                      Sets LogLevel to 'debug' in cupsd.conf. Highly recommended when diagnosing stuck filters or PPD issues.
                    </p>
                  </div>
                  <input
                    id="setting-debug-logging"
                    aria-describedby="setting-debug-logging-desc"
                    type="checkbox"
                    checked={settings.debug_logging}
                    onChange={(e) =>
                      setSettings({ ...settings, debug_logging: e.target.checked })
                    }
                    className="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 mt-1 cursor-pointer"
                  />
                </div>

                {/* PaperCut Info Callout */}
                <div className="p-3 rounded-xl bg-blue-50/70 dark:bg-blue-950/30 border border-blue-200/60 dark:border-blue-900/40 text-xs text-blue-800 dark:text-blue-300 leading-relaxed flex items-start space-x-2">
                  <ShieldCheck className="w-4 h-4 text-blue-600 dark:text-blue-400 shrink-0 mt-0.5" aria-hidden="true" />
                  <span>
                    Changes are applied to the live daemon instantly via <code>cupsctl</code> and survive service reloads.
                  </span>
                </div>

                {/* macOS Background Service (LaunchAgent) */}
                <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800 space-y-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-start space-x-3">
                      <Power className="w-5 h-5 text-indigo-600 dark:text-indigo-400 shrink-0 mt-0.5" aria-hidden="true" />
                      <div>
                        <div className="flex items-center gap-2">
                          <h4 className="text-xs font-semibold text-slate-900 dark:text-white block">
                            macOS Service (LaunchAgent)
                          </h4>
                          {launchAgentLoading ? (
                            <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-300">
                              Checking...
                            </span>
                          ) : launchAgent?.installed ? (
                            <span className="px-2 py-0.5 text-xs font-bold rounded-full bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800">
                              Active
                            </span>
                          ) : (
                            <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-300">
                              Not Installed
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed">
                          Registers <code>cups-webui</code> under <code>~/Library/LaunchAgents</code> so that <code>cups-webui open</code> starts the server. The server stops when you close the last browser tab.
                        </p>
                      </div>
                    </div>
                  </div>

                  {launchAgentMsg && (
                    <div
                      role="status"
                      aria-live="polite"
                      className="text-xs text-emerald-700 dark:text-emerald-300 bg-emerald-50 dark:bg-emerald-950/60 p-2 rounded-lg border border-emerald-200 dark:border-emerald-800"
                    >
                      {launchAgentMsg}
                    </div>
                  )}

                  {launchAgent?.supported && (
                    <div className="pt-2 border-t border-slate-200 dark:border-slate-700 flex items-center justify-between text-xs">
                      <span className="text-xs text-slate-500 dark:text-slate-400 font-mono truncate max-w-[240px]" title={launchAgent.plist_path}>
                        {launchAgent.plist_path.replace(/^.*\/LaunchAgents\//, '~/Library/LaunchAgents/')}
                      </span>
                      {launchAgent.installed ? (
                        <button
                          type="button"
                          disabled={launchAgentActionLoading}
                          onClick={() => handleManageLaunchAgent('uninstall')}
                          className="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg text-rose-700 dark:text-rose-300 bg-rose-50 dark:bg-rose-950/50 hover:bg-rose-100 dark:hover:bg-rose-900/50 border border-rose-200 dark:border-rose-800 transition disabled:opacity-50 cursor-pointer"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                          <span>Remove Service</span>
                        </button>
                      ) : (
                        <button
                          type="button"
                          disabled={launchAgentActionLoading}
                          onClick={() => handleManageLaunchAgent('install')}
                          className="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg text-indigo-700 dark:text-indigo-300 bg-indigo-50 dark:bg-indigo-950/50 hover:bg-indigo-100 dark:hover:bg-indigo-900/50 border border-indigo-200 dark:border-indigo-800 transition disabled:opacity-50 cursor-pointer"
                        >
                          <Power className="w-3.5 h-3.5" />
                          <span>Install Service</span>
                        </button>
                      )}
                    </div>
                  )}
                </div>

                {/* Optional SNMP hardware check */}
                {features && (
                  <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800">
                    <div className="flex items-start space-x-3">
                      <Radar className="w-5 h-5 text-indigo-600 dark:text-indigo-400 shrink-0 mt-0.5" aria-hidden="true" />
                      <div className="space-y-1">
                        <label htmlFor={snmpToggleId} className="flex items-center gap-2 text-xs font-semibold text-slate-900 dark:text-white cursor-pointer">
                          <input
                            id={snmpToggleId}
                            type="checkbox"
                            checked={features.snmp_enabled}
                            disabled={featuresSaving || !features.is_admin || (!features.snmp_available && !features.snmp_enabled)}
                            onChange={(e) => handleToggleSnmp(e.target.checked)}
                            className="rounded border-slate-300 dark:border-slate-600"
                          />
                          <span>SNMP hardware check (optional)</span>
                          {featuresSaving && <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" />}
                        </label>
                        <span className="text-xs text-slate-500 dark:text-slate-400 block">
                          Adds <em>Check installed hardware (SNMP)</em> to the Hardware Accessories tab. You type the printer's address and this Mac asks it over SNMP (UDP 161) which trays, bins and finishers are installed. Only works on a network that can reach the printers themselves. Off by default; saved as soon as you change it.
                        </span>
                        {!features.is_admin && (
                          <span className="text-xs text-amber-700 dark:text-amber-400 block">Only CUPS administrators can change this.</span>
                        )}
                        {!features.snmp_available && (
                          <span className="text-xs text-amber-700 dark:text-amber-400 block">snmpwalk isn't installed on this Mac.</span>
                        )}
                        {features.error && (
                          <span className="text-xs text-red-600 dark:text-red-400 block">{features.error}</span>
                        )}

                        {/* Print server list: fills in each queue's printer address */}
                        <div className="pt-2 mt-2 border-t border-slate-200 dark:border-slate-700 space-y-1.5">
                          <span className="text-xs font-semibold text-slate-900 dark:text-white block">Print server list (CSV)</span>
                          <span className="text-xs text-slate-500 dark:text-slate-400 block">
                            A printer list exported from a Windows print server (PrinterName, Model, Driver, PortName, IPAddress). The SNMP check uses it to fill in each queue's printer address. It stays on this Mac.
                          </span>
                          {inventory?.loaded ? (
                            <span className="text-xs text-slate-700 dark:text-slate-300 block">
                              {inventory.count} printers from <span className="font-mono">{inventory.source_name}</span>
                              {inventory.imported_at && <>, imported {new Date(inventory.imported_at).toLocaleString()}</>}
                            </span>
                          ) : (
                            <span className="text-xs text-slate-500 dark:text-slate-400 block">No list imported.</span>
                          )}
                          {inventory?.error && (
                            <span className="text-xs text-red-600 dark:text-red-400 block">{inventory.error}</span>
                          )}
                          <div className="flex items-center gap-2">
                            <label
                              htmlFor={csvInputId}
                              className={`inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700 ${inventoryBusy || !features.is_admin ? 'opacity-50 pointer-events-none' : 'cursor-pointer'}`}
                            >
                              {inventoryBusy ? <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" /> : <FileCode className="w-3.5 h-3.5" aria-hidden="true" />}
                              <span>{inventory?.loaded ? 'Replace list…' : 'Import CSV…'}</span>
                            </label>
                            <input
                              id={csvInputId}
                              type="file"
                              accept=".csv,text/csv"
                              className="sr-only"
                              disabled={inventoryBusy || !features.is_admin}
                              onChange={handleImportCsv}
                            />
                            {inventory?.loaded && (
                              <button
                                type="button"
                                onClick={handleRemoveCsv}
                                disabled={inventoryBusy || !features.is_admin}
                                className="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg text-red-700 dark:text-red-300 border border-red-200 dark:border-red-800 hover:bg-red-50 dark:hover:bg-red-950/40 disabled:opacity-50 cursor-pointer"
                              >
                                <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
                                <span>Remove list</span>
                              </button>
                            )}
                          </div>
                          {inventoryError && (
                            <span role="alert" className="text-xs text-red-600 dark:text-red-400 block">{inventoryError}</span>
                          )}
                          {importResult && (
                            <div role="status" className="text-xs text-slate-600 dark:text-slate-300 space-y-1">
                              <span className="block">
                                Imported {importResult.count} of {importResult.rows} rows.
                                {importResult.queues > 0 && <> {importResult.matched} of {importResult.queues} queues on this Mac found their printer.</>}
                              </span>
                              {importResult.skipped.length > 0 && (
                                <details>
                                  <summary className="cursor-pointer">
                                    Skipped {importResult.skipped.length} {importResult.skipped.length === 1 ? 'row' : 'rows'}
                                  </summary>
                                  <ul className="list-disc pl-4 max-h-32 overflow-y-auto">
                                    {importResult.skipped.map((s) => (
                                      <li key={s.line}>
                                        Line {s.line}{s.name ? ` (${s.name})` : ''}: {s.reason}
                                      </li>
                                    ))}
                                  </ul>
                                </details>
                              )}
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Footer */}
          <div className="px-6 py-4 bg-slate-50 dark:bg-slate-800/50 border-t border-slate-200 dark:border-slate-800 flex items-center justify-end space-x-2.5">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl text-xs font-semibold bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-750 transition cursor-pointer"
            >
              Close
            </button>
            <button
              type="submit"
              disabled={saving || loading || !settings}
              className="inline-flex items-center space-x-1.5 px-4 py-2 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-700 text-white shadow-sm transition disabled:opacity-50 cursor-pointer"
            >
              {saving ? (
                <>
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  <span>Applying...</span>
                </>
              ) : (
                <>
                  <Save className="w-3.5 h-3.5" />
                  <span>Apply Changes</span>
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
