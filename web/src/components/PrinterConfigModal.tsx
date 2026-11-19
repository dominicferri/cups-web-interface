import React, { useState, useEffect, useId } from 'react';
import {
  X,
  Sliders,
  Box,
  Cpu,
  CheckCircle2,
  AlertTriangle,
  Save,
  Loader2,
  Sparkles,
  Layers,
  Upload,
  Search,
  Network,
  Star,
} from 'lucide-react';
import type {
  Printer,
  PrinterOptionsResponse,
  ArchAuditResult,
  DriverInfo,
  PPDValidationResult,
  ModifyPrinterParams,
  AccessoryReport,
} from '../types/cups';
import { api } from '../api/client';
import { useModalDialog } from '../utils/useModalDialog';
import { AccessoriesPanel } from './AccessoriesPanel';
import { installedCount } from '../utils/accessories';

export type ConfigModalTab = 'preferences' | 'accessories' | 'audit' | 'details' | 'driver';

interface PrinterConfigModalProps {
  printer: Printer | null;
  initialTab?: ConfigModalTab;
  isOpen: boolean;
  onClose: () => void;
  onSaveSuccess?: () => void;
}

export const PrinterConfigModal: React.FC<PrinterConfigModalProps> = ({
  printer,
  initialTab = 'preferences',
  isOpen,
  onClose,
  onSaveSuccess,
}) => {
  const [activeTab, setActiveTab] = useState<ConfigModalTab>(initialTab);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [options, setOptions] = useState<PrinterOptionsResponse | null>(null);
  const [audit, setAudit] = useState<ArchAuditResult | null>(null);
  const [accessoryReport, setAccessoryReport] = useState<AccessoryReport | null>(null);
  // Bumped after a driver swap so the accessories tab remounts with the new PPD's options.
  const [accessoryPanelVersion, setAccessoryPanelVersion] = useState(0);
  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  // Queue Details Edit State
  const [detailsURI, setDetailsURI] = useState('');
  const [detailsDescription, setDetailsDescription] = useState('');
  const [detailsLocation, setDetailsLocation] = useState('');
  const [detailsShared, setDetailsShared] = useState(false);

  // Driver Swapping State
  const [driverCatalog, setDriverCatalog] = useState<DriverInfo[]>([]);
  const [driverSearch, setDriverSearch] = useState('');
  const [selectedDriver, setSelectedDriver] = useState<DriverInfo | null>(null);
  const [loadingDrivers, setLoadingDrivers] = useState(false);
  const [uploadedPPD, setUploadedPPD] = useState<File | null>(null);
  const [ppdValidation, setPpdValidation] = useState<PPDValidationResult | null>(null);
  const [validatingPPD, setValidatingPPD] = useState(false);
  const ppdFileInputId = useId();
  const titleId = useId();
  const dialogRef = useModalDialog(Boolean(isOpen && printer), onClose);

  useEffect(() => {
    if (initialTab) {
      setActiveTab(initialTab);
    }
  }, [initialTab]);

  useEffect(() => {
    if (!isOpen || !printer) return;

    setError(null);
    setSuccessMessage(null);
    setLoading(true);
    setAccessoryReport(null);

    // Seed queue details
    setDetailsURI(printer.uri || '');
    setDetailsDescription(printer.info || '');
    setDetailsLocation(printer.location || '');
    setDetailsShared(printer.is_shared || false);

    Promise.all([
      api.getPrinterOptions(printer.name).catch((err) => {
        console.error('Failed to load printer options:', err);
        return null;
      }),
      api.getPrinterAudit(printer.name).catch((err) => {
        console.error('Failed to load printer audit:', err);
        return null;
      }),
      api.getAccessories(printer.name).catch((err) => {
        console.error('Failed to load accessories:', err);
        return null;
      }),
    ])
      .then(([optsData, auditData, accessoryData]) => {
        setAccessoryReport(accessoryData);
        if (optsData) {
          applyOptions(optsData);
        }
        if (auditData) {
          setAudit(auditData);
        }
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : 'Failed to fetch printer configurations');
      })
      .finally(() => {
        setLoading(false);
      });
  }, [isOpen, printer]);

  function applyOptions(optsData: PrinterOptionsResponse) {
    setOptions(optsData);
    const initialValues: Record<string, string> = {};
    optsData.general.forEach((opt) => {
      initialValues[opt.key] = opt.current_value || opt.default_value;
    });
    optsData.accessories.forEach((opt) => {
      initialValues[opt.key] = opt.current_value || opt.default_value;
    });
    setFormValues(initialValues);
  }

  // Lazy load drivers when driver tab is selected
  useEffect(() => {
    if (activeTab === 'driver' && driverCatalog.length === 0) {
      loadDrivers('');
    }
  }, [activeTab, driverCatalog.length]);

  const loadDrivers = async (search: string) => {
    setLoadingDrivers(true);
    try {
      const list = await api.getDrivers(search);
      setDriverCatalog(list);
    } catch (err) {
      console.error('Failed to load drivers:', err);
    } finally {
      setLoadingDrivers(false);
    }
  };

  if (!isOpen || !printer) return null;

  const handleValueChange = (key: string, val: string) => {
    setFormValues((prev) => ({ ...prev, [key]: val }));
    setSuccessMessage(null);
  };

  // Save Options (Preferences or Accessories)
  const handleSaveOptions = async () => {
    if (!printer) return;
    setSaving(true);
    setError(null);
    setSuccessMessage(null);

    try {
      // Only general options: installable options are hardware settings saved
      // (with a warning and provenance) in the Hardware Accessories tab.
      const generalValues: Record<string, string> = {};
      options?.general.forEach((opt) => {
        if (formValues[opt.key] !== undefined) generalValues[opt.key] = formValues[opt.key];
      });
      await api.setPrinterOptions(printer.name, generalValues);
      setSuccessMessage('Printer options saved successfully!');
      if (onSaveSuccess) onSaveSuccess();
      setTimeout(() => setSuccessMessage(null), 3500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save options');
    } finally {
      setSaving(false);
    }
  };

  // Save Queue Details
  const handleSaveDetails = async () => {
    if (!printer) return;
    setSaving(true);
    setError(null);
    setSuccessMessage(null);

    try {
      const params: ModifyPrinterParams = {
        uri: detailsURI.trim(),
        description: detailsDescription.trim(),
        location: detailsLocation.trim(),
        shared: detailsShared,
      };
      await api.modifyPrinter(printer.name, params);
      setSuccessMessage('Queue details updated successfully!');
      if (onSaveSuccess) onSaveSuccess();
      setTimeout(() => setSuccessMessage(null), 3500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update queue details');
    } finally {
      setSaving(false);
    }
  };

  // Handle PPD Upload
  const handlePPDFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setUploadedPPD(file);
    setValidatingPPD(true);
    setPpdValidation(null);
    setError(null);

    try {
      const res = await api.validatePPD(file);
      setPpdValidation(res);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to validate PPD file';
      setError(msg);
    } finally {
      setValidatingPPD(false);
    }
  };

  // Swap Driver
  const handleSwapDriver = async () => {
    if (!printer) return;
    setSaving(true);
    setError(null);
    setSuccessMessage(null);

    try {
      const targetDriver = selectedDriver ? selectedDriver.name : (uploadedPPD ? uploadedPPD.name : '');
      if (!targetDriver) {
        setError('Please select a driver from the catalog or upload a custom PPD.');
        setSaving(false);
        return;
      }
      await api.setPrinterDriver(printer.name, targetDriver);
      setSuccessMessage(`Driver updated to "${targetDriver}"!`);
      // The new PPD changes the audit, the default options and the
      // accessories (manual values were re-applied by the server): reload all.
      const [updatedAudit, updatedOptions, updatedAccessories] = await Promise.all([
        api.getPrinterAudit(printer.name),
        api.getPrinterOptions(printer.name).catch(() => null),
        api.getAccessories(printer.name).catch(() => null),
      ]);
      setAudit(updatedAudit);
      if (updatedOptions) applyOptions(updatedOptions);
      setAccessoryReport(updatedAccessories);
      setAccessoryPanelVersion((v) => v + 1);
      if (onSaveSuccess) onSaveSuccess();
      setTimeout(() => setSuccessMessage(null), 3500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update driver');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 overflow-hidden bg-slate-900/60 backdrop-blur-xs transition-opacity animate-in fade-in duration-200">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="fixed inset-y-0 right-0 max-w-2xl w-full bg-white dark:bg-slate-900 shadow-2xl flex flex-col animate-in slide-in-from-right duration-250 border-l border-slate-200 dark:border-slate-800 focus:outline-none"
      >
        
        {/* Header */}
        <div className="px-6 py-5 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between shrink-0 bg-white/95 dark:bg-slate-900/95 backdrop-blur-md">
          <div className="flex items-center space-x-3">
            <div className="p-2.5 bg-blue-50 dark:bg-blue-950/60 text-blue-600 dark:text-blue-400 rounded-2xl">
              <Sliders className="w-5 h-5" aria-hidden="true" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <h2 id={titleId} className="text-base font-bold text-slate-900 dark:text-white">
                  {printer.name}
                </h2>
                <span className="text-xs px-2 py-0.5 rounded-full font-medium bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300">
                  CUPS Queue
                </span>
              </div>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 truncate max-w-sm">
                {printer.info || printer.name}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-2 text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
            aria-label="Close dialog"
          >
            <X className="w-5 h-5" aria-hidden="true" />
          </button>
        </div>

        {/* Tab Navigation */}
        <div
          role="tablist"
          aria-label="Queue configuration sections"
          className="flex overflow-x-auto border-b border-slate-200 dark:border-slate-800 bg-slate-50/75 dark:bg-slate-900/50 px-6 pt-2 shrink-0"
        >
          <button
            type="button"
            role="tab"
            id="config-tab-preferences"
            aria-selected={activeTab === 'preferences'}
            aria-controls="config-tabpanel-preferences"
            onClick={() => setActiveTab('preferences')}
            className={`flex items-center space-x-2 py-2.5 px-3 text-xs font-semibold border-b-2 transition -mb-px shrink-0 cursor-pointer ${
              activeTab === 'preferences'
                ? 'border-blue-600 text-slate-900 dark:text-white dark:border-blue-400 font-bold'
                : 'border-transparent text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white'
            }`}
          >
            <Sliders className={`w-3.5 h-3.5 ${activeTab === 'preferences' ? 'text-blue-600 dark:text-blue-400' : 'text-slate-500 dark:text-slate-400'}`} aria-hidden="true" />
            <span>Default Options</span>
          </button>

          <button
            type="button"
            role="tab"
            id="config-tab-accessories"
            aria-selected={activeTab === 'accessories'}
            aria-controls="config-tabpanel-accessories"
            onClick={() => setActiveTab('accessories')}
            className={`flex items-center space-x-2 py-2.5 px-3 text-xs font-semibold border-b-2 transition -mb-px shrink-0 cursor-pointer ${
              activeTab === 'accessories'
                ? 'border-blue-600 text-slate-900 dark:text-white dark:border-blue-400 font-bold'
                : 'border-transparent text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white'
            }`}
          >
            <Box className={`w-3.5 h-3.5 ${activeTab === 'accessories' ? 'text-blue-600 dark:text-blue-400' : 'text-slate-500 dark:text-slate-400'}`} aria-hidden="true" />
            <span>Hardware Accessories</span>
            {installedCount(accessoryReport) !== null && (
              <span
                title={`${installedCount(accessoryReport)} of ${accessoryReport?.options.length} accessories installed`}
                className="ml-1 px-1.5 py-0.2 rounded-full text-xs font-bold bg-blue-100 dark:bg-slate-700 text-blue-800 dark:text-white border border-blue-200 dark:border-slate-600"
              >
                {installedCount(accessoryReport)}
              </span>
            )}
          </button>

          <button
            type="button"
            role="tab"
            id="config-tab-details"
            aria-selected={activeTab === 'details'}
            aria-controls="config-tabpanel-details"
            onClick={() => setActiveTab('details')}
            className={`flex items-center space-x-2 py-2.5 px-3 text-xs font-semibold border-b-2 transition -mb-px shrink-0 cursor-pointer ${
              activeTab === 'details'
                ? 'border-blue-600 text-slate-900 dark:text-white dark:border-blue-400 font-bold'
                : 'border-transparent text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white'
            }`}
          >
            <Network className={`w-3.5 h-3.5 ${activeTab === 'details' ? 'text-blue-600 dark:text-blue-400' : 'text-slate-500 dark:text-slate-400'}`} aria-hidden="true" />
            <span>Queue Details</span>
          </button>

          <button
            type="button"
            role="tab"
            id="config-tab-driver"
            aria-selected={activeTab === 'driver'}
            aria-controls="config-tabpanel-driver"
            onClick={() => setActiveTab('driver')}
            className={`flex items-center space-x-2 py-2.5 px-3 text-xs font-semibold border-b-2 transition -mb-px shrink-0 cursor-pointer ${
              activeTab === 'driver'
                ? 'border-blue-600 text-slate-900 dark:text-white dark:border-blue-400 font-bold'
                : 'border-transparent text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white'
            }`}
          >
            <Layers className={`w-3.5 h-3.5 ${activeTab === 'driver' ? 'text-blue-600 dark:text-blue-400' : 'text-slate-500 dark:text-slate-400'}`} aria-hidden="true" />
            <span>Swap Driver</span>
          </button>

          <button
            type="button"
            role="tab"
            id="config-tab-audit"
            aria-selected={activeTab === 'audit'}
            aria-controls="config-tabpanel-audit"
            onClick={() => setActiveTab('audit')}
            className={`flex items-center space-x-2 py-2.5 px-3 text-xs font-semibold border-b-2 transition -mb-px shrink-0 cursor-pointer ${
              activeTab === 'audit'
                ? 'border-blue-600 text-slate-900 dark:text-white dark:border-blue-400 font-bold'
                : 'border-transparent text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white'
            }`}
          >
            <Cpu className={`w-3.5 h-3.5 ${activeTab === 'audit' ? 'text-blue-600 dark:text-blue-400' : 'text-slate-500 dark:text-slate-400'}`} aria-hidden="true" />
            <span>Rosetta 2 &amp; Audit</span>
            {printer.arch_readiness?.rosetta_required && (
              <span className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" aria-hidden="true" />
            )}
          </button>
        </div>

        {/* Status Alerts */}
        {error && (
          <div
            role="alert"
            className="mx-6 mt-4 p-3 rounded-xl bg-rose-50 dark:bg-rose-950/50 border border-rose-200 dark:border-rose-800 text-rose-700 dark:text-rose-300 text-xs flex items-center space-x-2"
          >
            <AlertTriangle className="w-4 h-4 shrink-0" aria-hidden="true" />
            <span>{error}</span>
          </div>
        )}
        {successMessage && (
          <div
            role="status"
            aria-live="polite"
            className="mx-6 mt-4 p-3 rounded-xl bg-emerald-50 dark:bg-emerald-950/50 border border-emerald-200 dark:border-emerald-800 text-emerald-700 dark:text-emerald-300 text-xs flex items-center space-x-2"
          >
            <CheckCircle2 className="w-4 h-4 shrink-0" aria-hidden="true" />
            <span>{successMessage}</span>
          </div>
        )}

        {/* Tab Content Body */}
        <div
          id={`config-tabpanel-${activeTab}`}
          role="tabpanel"
          aria-labelledby={`config-tab-${activeTab}`}
          className="p-6 overflow-y-auto flex-1 space-y-6"
        >
          {loading ? (
            <div className="flex flex-col items-center justify-center py-12 text-slate-500 dark:text-slate-400 space-y-2">
              <Loader2 className="w-6 h-6 animate-spin text-blue-600" aria-hidden="true" />
              <p className="text-xs">Inspecting PPD driver and hardware options...</p>
            </div>
          ) : activeTab === 'preferences' ? (
            /* Tab 1: Default Options */
            <div className="space-y-5">
              {options?.general && options.general.length > 0 ? (
                options.general.map((opt) => {
                  const currentValue = formValues[opt.key] || opt.default_value;
                  const selectId = `pref-opt-${opt.key}`;
                  return (
                    <div
                      key={opt.key}
                      className="p-4 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-850/60 shadow-2xs space-y-2"
                    >
                      <div className="flex items-center justify-between">
                        <label
                          htmlFor={selectId}
                          className="text-xs font-semibold text-slate-800 dark:text-slate-200"
                        >
                          {opt.display_name}
                        </label>
                        <span className="text-xs font-mono text-slate-500 dark:text-slate-400">
                          {opt.key}
                        </span>
                      </div>

                      <select
                        id={selectId}
                        value={currentValue}
                        onChange={(e) => handleValueChange(opt.key, e.target.value)}
                        className="w-full px-3 py-2 text-xs rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-slate-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                      >
                        {opt.choices.map((choice) => (
                          <option key={choice.name} value={choice.name}>
                            {choice.label} {choice.name === opt.default_value ? '(Factory Default)' : ''}
                          </option>
                        ))}
                      </select>
                    </div>
                  );
                })
              ) : (
                <div className="text-center py-10 text-slate-500 dark:text-slate-400 text-xs">
                  No configurable PPD default options found for this print queue.
                </div>
              )}
            </div>
          ) : activeTab === 'accessories' ? (
            /* Tab 2: Hardware Accessories (installable options) */
            <AccessoriesPanel
              key={`${printer.name}-${accessoryPanelVersion}`}
              printerName={printer.name}
              initialReport={accessoryReport}
              onChanged={onSaveSuccess}
              onReport={setAccessoryReport}
            />
          ) : activeTab === 'details' ? (
            /* Tab 3: Queue Details */
            <div className="space-y-4">
              <div>
                <label
                  htmlFor="config-queue-name"
                  className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                >
                  Queue Name (CUPS Identifier)
                </label>
                <input
                  id="config-queue-name"
                  type="text"
                  value={printer.name}
                  disabled
                  className="w-full px-3 py-2 rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-100 dark:bg-slate-800/60 text-xs text-slate-500 dark:text-slate-400 font-mono cursor-not-allowed"
                />
              </div>

              <div>
                <label
                  htmlFor="config-device-uri"
                  className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                >
                  Device Connection URI
                </label>
                <input
                  id="config-device-uri"
                  type="text"
                  value={detailsURI}
                  onChange={(e) => setDetailsURI(e.target.value)}
                  placeholder="socket://192.168.1.50:9100"
                  className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>

              <div>
                <label
                  htmlFor="config-description"
                  className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                >
                  Description / Human Friendly Name
                </label>
                <input
                  id="config-description"
                  type="text"
                  value={detailsDescription}
                  onChange={(e) => setDetailsDescription(e.target.value)}
                  placeholder="e.g. 2nd Floor Color Copier"
                  className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>

              <div>
                <label
                  htmlFor="config-location"
                  className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                >
                  Physical Location
                </label>
                <input
                  id="config-location"
                  type="text"
                  value={detailsLocation}
                  onChange={(e) => setDetailsLocation(e.target.value)}
                  placeholder="e.g. Room 204"
                  className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>

              <div className="pt-2">
                <label className="inline-flex items-center space-x-2.5 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={detailsShared}
                    onChange={(e) => setDetailsShared(e.target.checked)}
                    className="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer"
                  />
                  <div>
                    <span className="text-xs font-semibold text-slate-800 dark:text-slate-200">
                      Share printer queue via Bonjour / mDNS
                    </span>
                    <p className="text-xs text-slate-500 dark:text-slate-400">
                      Controls CUPS <code>-u allow:all</code> vs <code>-u default:deny</code> publishing.
                    </p>
                  </div>
                </label>
              </div>

              <div className="pt-3 border-t border-slate-200 dark:border-slate-700">
                <div className="flex items-center justify-between">
                  <div>
                    <span className="text-xs font-semibold text-slate-800 dark:text-slate-200 block">
                      System Default Destination
                    </span>
                    <p className="text-xs text-slate-500 dark:text-slate-400">
                      Target queue used when print jobs don't specify a printer destination.
                    </p>
                  </div>
                  {printer.is_default ? (
                    <span className="inline-flex items-center space-x-1 px-2.5 py-1 rounded-full text-xs font-semibold bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300 border border-blue-200 dark:border-blue-800 shrink-0">
                      <Star className="w-3 h-3 fill-current" aria-hidden="true" />
                      <span>Current Default</span>
                    </span>
                  ) : (
                    <button
                      type="button"
                      onClick={async () => {
                        try {
                          await api.setDefaultPrinter(printer.name);
                          if (onSaveSuccess) onSaveSuccess();
                        } catch (err) {
                          setError(err instanceof Error ? err.message : 'Failed to set default printer');
                        }
                      }}
                      className="px-3 py-1.5 text-xs font-semibold rounded-xl bg-slate-100 hover:bg-slate-200 dark:bg-slate-700 dark:hover:bg-slate-600 text-slate-700 dark:text-slate-200 border border-slate-300 dark:border-slate-600 cursor-pointer flex items-center space-x-1.5 transition"
                    >
                      <Star className="w-3.5 h-3.5 text-amber-500" aria-hidden="true" />
                      <span>Set as Default</span>
                    </button>
                  )}
                </div>
              </div>
            </div>
          ) : activeTab === 'driver' ? (
            /* Tab 4: Swap Driver / PPD */
            <div className="space-y-4">
              <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 flex items-start justify-between gap-4 text-xs">
                <div className="min-w-0 space-y-1">
                  <span className="text-slate-500 dark:text-slate-400 text-xs uppercase tracking-wider font-bold block">
                    Current Driver in Use
                  </span>
                  <span className="text-sm font-bold text-slate-900 dark:text-white block">
                    {audit?.driver_model || printer.arch_readiness?.driver_model || 'PostScript Printer Driver (PPD)'}
                  </span>
                  {(audit?.ppd_path || printer.arch_readiness?.ppd_path) && (
                    <span className="text-xs font-mono text-slate-500 dark:text-slate-400 block truncate">
                      {audit?.ppd_path || printer.arch_readiness?.ppd_path}
                    </span>
                  )}
                </div>
                <div className="shrink-0 pt-0.5">
                  {(audit?.rosetta_required ?? printer.arch_readiness?.rosetta_required) ? (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-semibold bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300 border border-amber-300 dark:border-amber-800">
                      <AlertTriangle className="w-3 h-3 shrink-0" aria-hidden="true" />
                      <span>Intel only</span>
                    </span>
                  ) : (
                    <span className="text-xs text-slate-500 dark:text-slate-400">
                      {(audit?.status || printer.arch_readiness?.status) === 'driverless' ? 'Driverless' : 'Universal'}
                    </span>
                  )}
                </div>
              </div>

              {/* Driver search input */}
              <div className="relative">
                <label htmlFor="config-driver-search" className="sr-only">
                  Search installed drivers by make, model, or file
                </label>
                <Search className="w-4 h-4 text-slate-500 dark:text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" aria-hidden="true" />
                <input
                  id="config-driver-search"
                  type="text"
                  value={driverSearch}
                  onChange={(e) => {
                    setDriverSearch(e.target.value);
                    loadDrivers(e.target.value);
                  }}
                  placeholder="Search installed drivers by make, model, or file..."
                  aria-label="Search installed drivers by make, model, or file"
                  className="w-full pl-9 pr-4 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>

              {/* Drivers list */}
              <div className="max-h-56 overflow-y-auto rounded-xl border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
                {loadingDrivers ? (
                  <div className="p-4 text-center text-xs text-slate-500 dark:text-slate-400 flex items-center justify-center space-x-2">
                    <Loader2 className="w-4 h-4 animate-spin text-blue-500" aria-hidden="true" />
                    <span>Searching drivers...</span>
                  </div>
                ) : driverCatalog.length === 0 ? (
                  <div className="p-4 text-center text-xs text-slate-500 dark:text-slate-400">
                    No drivers found.
                  </div>
                ) : (
                  driverCatalog.map((d) => {
                    const isSelected = selectedDriver?.name === d.name;
                    return (
                      <button
                        key={d.name}
                        type="button"
                        aria-pressed={isSelected}
                        onClick={() => setSelectedDriver(d)}
                        className={`w-full text-left p-2.5 flex items-center justify-between text-xs transition cursor-pointer ${
                          isSelected
                            ? 'bg-blue-50/80 dark:bg-blue-950/40 text-blue-900 dark:text-blue-100'
                            : 'hover:bg-slate-50 dark:hover:bg-slate-800/60 text-slate-700 dark:text-slate-300'
                        }`}
                      >
                        <div className="truncate pr-3">
                          <div className="font-semibold text-slate-900 dark:text-white truncate">
                            {d.model}
                          </div>
                          <div className="text-xs font-mono text-slate-500 dark:text-slate-400 truncate">
                            {d.name}
                          </div>
                        </div>

                        <div className="shrink-0">
                          {d.arch_readiness.status === 'rosetta_dependent' ? (
                            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-semibold bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300 border border-amber-300 dark:border-amber-800">
                              <AlertTriangle className="w-3 h-3 shrink-0" aria-hidden="true" />
                              <span>Intel only</span>
                            </span>
                          ) : (
                            <span className="text-xs text-slate-500 dark:text-slate-400">
                              {d.arch_readiness.status === 'driverless' ? 'Driverless' : 'Universal'}
                            </span>
                          )}
                        </div>
                      </button>
                    );
                  })
                )}
              </div>

              {/* Or upload custom PPD */}
              <div className="pt-2 border-t border-slate-100 dark:border-slate-800">
                <label
                  htmlFor={ppdFileInputId}
                  className="border border-dashed border-slate-300 dark:border-slate-700 hover:border-blue-500 rounded-xl p-3 text-center flex items-center justify-center space-x-2 cursor-pointer transition bg-slate-50/50 dark:bg-slate-800/30"
                >
                  <input
                    id={ppdFileInputId}
                    type="file"
                    accept=".ppd,.ppd.gz,.gz"
                    onChange={handlePPDFileChange}
                    className="sr-only"
                  />
                  <Upload className="w-4 h-4 text-blue-600 dark:text-blue-400" aria-hidden="true" />
                  <span className="text-xs text-slate-700 dark:text-slate-300">
                    {uploadedPPD ? uploadedPPD.name : 'Or upload custom .ppd file'}
                  </span>
                </label>

                {validatingPPD && (
                  <div className="p-2 text-center text-xs text-slate-500 dark:text-slate-400 flex items-center justify-center space-x-2">
                    <Loader2 className="w-3.5 h-3.5 animate-spin text-blue-500" aria-hidden="true" />
                    <span>Validating PPD syntax...</span>
                  </div>
                )}

                {ppdValidation && (
                  <div
                    role={ppdValidation.valid ? 'status' : 'alert'}
                    className={`p-3 rounded-xl border text-xs space-y-1 ${
                      ppdValidation.valid
                        ? 'bg-emerald-50 dark:bg-emerald-950/40 border-emerald-200 dark:border-emerald-800 text-emerald-900 dark:text-emerald-200'
                        : 'bg-rose-50 dark:bg-rose-950/40 border-rose-200 dark:border-rose-800 text-rose-900 dark:text-rose-200'
                    }`}
                  >
                    <div className="flex items-center space-x-1.5 font-semibold">
                      {ppdValidation.valid ? (
                        <>
                          <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" aria-hidden="true" />
                          <span>Valid PPD: {ppdValidation.model}</span>
                        </>
                      ) : (
                        <>
                          <AlertTriangle className="w-3.5 h-3.5 text-rose-600 dark:text-rose-400" aria-hidden="true" />
                          <span>PPD Validation Failed</span>
                        </>
                      )}
                    </div>
                    {ppdValidation.errors && ppdValidation.errors.length > 0 && (
                      <p className="text-xs text-rose-700 dark:text-rose-300 font-mono">
                        {ppdValidation.errors[0]}
                      </p>
                    )}
                  </div>
                )}
              </div>
            </div>
          ) : (
            /* Tab 5: Mach-O Architecture & Rosetta 2 Audit */
            <div className="space-y-5">
              {audit ? (
                <>
                  {/* Current Driver Callout */}
                  <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 flex items-start justify-between gap-4">
                    <div className="min-w-0 space-y-1">
                      <span className="text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400 block">
                        Current Driver in Use
                      </span>
                      <span className="text-sm font-bold text-slate-900 dark:text-white block">
                        {audit.driver_model || 'PostScript Printer Driver (PPD)'}
                      </span>
                      {audit.ppd_path ? (
                        <span className="text-xs font-mono text-slate-500 dark:text-slate-400 block truncate">
                          {audit.ppd_path}
                        </span>
                      ) : (
                        <span className="text-xs text-slate-500 dark:text-slate-400 block">
                          No local PPD file (Driverless / System Proxy)
                        </span>
                      )}
                    </div>

                    <button
                      type="button"
                      onClick={() => setActiveTab('driver')}
                      className="shrink-0 text-xs font-semibold text-blue-600 dark:text-blue-400 hover:underline cursor-pointer"
                    >
                      Swap Driver →
                    </button>
                  </div>

                  <div
                    className={`p-4 rounded-2xl border ${
                      audit.rosetta_required
                        ? 'bg-amber-50/80 dark:bg-amber-950/30 border-amber-200 dark:border-amber-800'
                        : 'bg-emerald-50/80 dark:bg-emerald-950/30 border-emerald-200 dark:border-emerald-800'
                    }`}
                  >
                    <div className="flex items-center space-x-3">
                      <div
                        className={`p-2.5 rounded-xl ${
                          audit.rosetta_required
                            ? 'bg-amber-100 dark:bg-amber-900/60 text-amber-700 dark:text-amber-300'
                            : 'bg-emerald-100 dark:bg-emerald-900/60 text-emerald-700 dark:text-emerald-300'
                        }`}
                      >
                        <Cpu className="w-5 h-5" aria-hidden="true" />
                      </div>
                      <div>
                        <h4 className="text-sm font-bold text-slate-900 dark:text-white">
                          {audit.label}
                        </h4>
                        <p className="text-xs text-slate-600 dark:text-slate-300 mt-0.5">
                          {audit.migration_advice}
                        </p>
                      </div>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <h5 className="text-xs font-bold text-slate-500 dark:text-slate-400 uppercase tracking-wider">
                      Compiled Mach-O Binaries &amp; Architecture Slices
                    </h5>
                    {audit.binaries && audit.binaries.length > 0 ? (
                      audit.binaries.map((bin) => (
                        <div
                          key={bin.path}
                          className="p-3.5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-850/60 text-xs space-y-1.5"
                        >
                          <div className="flex items-center justify-between">
                            <span className="font-semibold text-slate-900 dark:text-white">
                              {bin.name}
                            </span>
                            <div className="flex items-center space-x-1">
                              {bin.slices.map((slice) => (
                                <span
                                  key={slice}
                                  className="px-1.5 py-0.5 rounded text-xs font-mono font-semibold bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300"
                                >
                                  {slice}
                                </span>
                              ))}
                            </div>
                          </div>
                          <p className="text-xs font-mono text-slate-500 dark:text-slate-400 truncate">
                            {bin.path}
                          </p>
                          <p className="text-xs text-slate-500 dark:text-slate-400">
                            {bin.recommendation}
                          </p>
                        </div>
                      ))
                    ) : (
                      <div className="p-4 rounded-xl border border-dashed border-slate-200 dark:border-slate-700 text-center text-xs text-slate-500 dark:text-slate-400">
                        <Sparkles className="w-5 h-5 mx-auto mb-1 text-sky-500" aria-hidden="true" />
                        No external vendor filter binaries required. Uses native system PDF rasterization.
                      </div>
                    )}
                  </div>
                </>
              ) : (
                <div className="text-center py-8 text-slate-500 dark:text-slate-400 text-xs">
                  No architecture audit data available.
                </div>
              )}
            </div>
          )}
        </div>

        {/* Modal Footer */}
        <div className="px-6 py-4 bg-slate-50 dark:bg-slate-900/80 border-t border-slate-200 dark:border-slate-800 flex items-center justify-between shrink-0">
          <span className="text-xs text-slate-500 dark:text-slate-400">
            {activeTab === 'audit'
              ? 'Audit verified via debug/macho'
              : activeTab === 'accessories'
                ? 'Installable options are read from the queue PPD in CUPS'
                : 'Changes apply immediately to CUPS queue'}
          </span>

          <div className="flex items-center space-x-2.5">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-800 transition cursor-pointer"
            >
              Close
            </button>
            {activeTab === 'details' && (
              <button
                type="button"
                onClick={handleSaveDetails}
                disabled={saving}
                className="inline-flex items-center space-x-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 transition shadow-xs cursor-pointer"
              >
                {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
                <span>Save Queue Details</span>
              </button>
            )}
            {activeTab === 'driver' && (
              <button
                type="button"
                onClick={handleSwapDriver}
                disabled={saving || (!selectedDriver && !uploadedPPD)}
                className="inline-flex items-center space-x-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 transition shadow-xs cursor-pointer"
              >
                {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Layers className="w-3.5 h-3.5" />}
                <span>Apply Driver Swap</span>
              </button>
            )}
            {activeTab === 'preferences' && (
              <button
                type="button"
                onClick={handleSaveOptions}
                disabled={saving || loading}
                className="inline-flex items-center space-x-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 transition shadow-xs cursor-pointer"
              >
                {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
                <span>Save Default Options</span>
              </button>
            )}
          </div>
        </div>

      </div>
    </div>
  );
};
