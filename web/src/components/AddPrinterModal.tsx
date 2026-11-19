import React, { useState, useEffect, useId } from 'react';
import {
  X,
  Printer as PrinterIcon,
  CheckCircle2,
  AlertTriangle,
  AlertCircle,
  Upload,
  Search,
  FileText,
  Sliders,
  ShieldCheck,
  Loader2,
  Network,
  ArrowRight,
} from 'lucide-react';
import type { Printer, DriverInfo, PPDValidationResult, AddPrinterParams } from '../types/cups';
import { api } from '../api/client';
import { useModalDialog } from '../utils/useModalDialog';

interface AddPrinterModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: (printerName: string) => void;
  onConfigureCreated: (printer: Printer) => void;
}

type ProtocolType = 'socket' | 'ipp' | 'ipps' | 'lpd' | 'everywhere' | 'custom';

export const AddPrinterModal: React.FC<AddPrinterModalProps> = ({
  isOpen,
  onClose,
  onSuccess,
  onConfigureCreated,
}) => {
  // Protocol State - AppSocket/HP JetDirect is standard default
  const [protocol, setProtocol] = useState<ProtocolType>('socket');
  const [host, setHost] = useState('');
  const [port, setPort] = useState('9100');
  const [queuePath, setQueuePath] = useState('ipp/print');
  const [customURI, setCustomURI] = useState('');

  // Queue Details State - Safe naming enabled by default
  const [safeNaming, setSafeNaming] = useState(true);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [location, setLocation] = useState('');
  const [shared, setShared] = useState(false); // Default to unshared

  // Driver / PPD State
  const [driverMode, setDriverMode] = useState<'catalog' | 'upload'>('catalog');
  const [drivers, setDrivers] = useState<DriverInfo[]>([]);
  const [driverSearch, setDriverSearch] = useState('');
  const [selectedDriver, setSelectedDriver] = useState<DriverInfo | null>(null);
  const [loadingDrivers, setLoadingDrivers] = useState(false);

  // Custom PPD Upload State
  const [uploadedFile, setUploadedFile] = useState<File | null>(null);
  const [ppdValidation, setPpdValidation] = useState<PPDValidationResult | null>(null);
  const [validatingPPD, setValidatingPPD] = useState(false);

  // Creation State
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createdPrinterName, setCreatedPrinterName] = useState<string | null>(null);

  const fileInputId = useId();
  const titleId = useId();
  const dialogRef = useModalDialog(isOpen, onClose);

  // Reset state when modal opens
  useEffect(() => {
    if (isOpen) {
      setProtocol('socket');
      setHost('');
      setPort('9100');
      setQueuePath('ipp/print');
      setCustomURI('');
      setSafeNaming(true);
      setName('');
      setDescription('');
      setLocation('');
      setShared(false);
      setDriverMode('catalog');
      setSelectedDriver(null);
      setUploadedFile(null);
      setPpdValidation(null);
      setError(null);
      setCreatedPrinterName(null);
      loadDrivers('');
    }
  }, [isOpen]);

  // Load driver catalog
  const loadDrivers = async (search: string) => {
    setLoadingDrivers(true);
    try {
      const data = await api.getDrivers(search);
      setDrivers(data);
      if (!selectedDriver && data.length > 0) {
        const preferred = data.find((d) => d.name.includes('laserjet') || d.name.includes('generic')) || data[0];
        setSelectedDriver(preferred);
      }
    } catch (err) {
      console.error('Failed to load drivers:', err);
    } finally {
      setLoadingDrivers(false);
    }
  };

  // Sync description to name when in Safe Naming mode
  const handleNameChange = (val: string) => {
    if (safeNaming) {
      // Strictly alphanumeric characters
      const sanitized = val.replace(/[^a-zA-Z0-9]/g, '');
      setName(sanitized);
      setDescription(sanitized);
    } else {
      // Standard CUPS queue name restrictions: no space, no / \ ' " ? #
      const sanitized = val.replace(/[\s/\\'"?#]/g, '_');
      setName(sanitized);
    }
  };

  // Compute final URI
  const computeURI = (): string => {
    const cleanHost = host.trim();
    if (!cleanHost && protocol !== 'custom' && protocol !== 'everywhere') {
      return '';
    }
    switch (protocol) {
      case 'socket':
        return `socket://${cleanHost}:${port || '9100'}`;
      case 'ipp':
        return `ipp://${cleanHost}:${port || '631'}/${queuePath.replace(/^\//, '')}`;
      case 'ipps':
        return `ipps://${cleanHost}:${port || '443'}/${queuePath.replace(/^\//, '')}`;
      case 'lpd':
        return `lpd://${cleanHost}/${queuePath.replace(/^\//, '') || 'raw'}`;
      case 'everywhere':
        return cleanHost ? `ipp://${cleanHost}/ipp/print` : 'ipp://printer.local/ipp/print';
      case 'custom':
        return customURI.trim();
    }
  };

  // Handle custom PPD file selection
  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setUploadedFile(file);
    setValidatingPPD(true);
    setPpdValidation(null);
    setError(null);

    try {
      const result = await api.validatePPD(file);
      setPpdValidation(result);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to validate PPD file';
      setError(msg);
    } finally {
      setValidatingPPD(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    const finalName = name.trim();
    if (!finalName) {
      setError('Queue name is required.');
      return;
    }

    if (safeNaming && !/^[a-zA-Z0-9]+$/.test(finalName)) {
      setError('With Safe Naming enabled, Queue Name must contain ONLY alphanumeric characters (A-Z, 0-9).');
      return;
    }

    const finalURI = computeURI();
    if (!finalURI) {
      setError('Please provide a valid printer IP or hostname to construct the connection URI.');
      return;
    }

    if (driverMode === 'upload' && (!ppdValidation || !ppdValidation.valid)) {
      setError('A valid custom PPD file must be uploaded and pass Adobe specification checks.');
      return;
    }

    setSubmitting(true);
    try {
      const params: AddPrinterParams = {
        name: finalName,
        description: safeNaming ? finalName : (description.trim() || finalName),
        uri: finalURI,
        location: location.trim(),
        shared: shared,
        model: driverMode === 'catalog' && selectedDriver ? selectedDriver.name : undefined,
        make: driverMode === 'catalog' && selectedDriver ? selectedDriver.make : undefined,
      };

      await api.addPrinter(params);
      setCreatedPrinterName(finalName);
      onSuccess(finalName);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to create printer queue';
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  const handleOpenConfigForCreated = () => {
    if (!createdPrinterName) return;
    const dummyPrinter: Printer = {
      name: createdPrinterName,
      uri: computeURI(),
      info: safeNaming ? createdPrinterName : description || createdPrinterName,
      location: location,
      state: 'idle',
      state_message: 'Ready to print',
      is_default: false,
      is_shared: shared,
      is_class: false,
      accepting_jobs: true,
      queued_jobs_count: 0,
    };
    onClose();
    onConfigureCreated(dummyPrinter);
  };

  if (!isOpen) return null;

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
        
        {/* Sticky Header */}
        <div className="px-6 py-5 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between shrink-0 bg-white/95 dark:bg-slate-900/95 backdrop-blur-md">
          <div className="flex items-center space-x-3">
            <div className="p-2.5 bg-blue-50 dark:bg-blue-950/60 text-blue-600 dark:text-blue-400 rounded-2xl">
              <PrinterIcon className="w-6 h-6" aria-hidden="true" />
            </div>
            <div>
              <h2 id={titleId} className="text-lg font-bold text-slate-900 dark:text-white leading-tight">
                Add New Print Queue
              </h2>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                Configure network connectivity, queue naming, and driver / PPD
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-2 text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
            title="Close"
            aria-label="Close dialog"
          >
            <X className="w-5 h-5" aria-hidden="true" />
          </button>
        </div>

        {/* Modal Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-6">
          
          {/* Post-Creation Immediate Prompt View */}
          {createdPrinterName ? (
            <div className="py-6 space-y-6 animate-in fade-in duration-200">
              <div className="text-center space-y-3">
                <div className="w-16 h-16 mx-auto bg-emerald-100 dark:bg-emerald-950/60 text-emerald-600 dark:text-emerald-400 rounded-3xl flex items-center justify-center">
                  <CheckCircle2 className="w-9 h-9" aria-hidden="true" />
                </div>
                <h3 className="text-xl font-bold text-slate-900 dark:text-white">
                  Queue "{createdPrinterName}" Created!
                </h3>
                <p className="text-sm text-slate-600 dark:text-slate-300 max-w-md mx-auto">
                  The print queue is now registered in the CUPS subsystem and accepting jobs.
                </p>
              </div>

              {/* Recommended Step Box */}
              <div className="bg-blue-50 dark:bg-blue-950/40 border border-blue-200 dark:border-blue-800/80 rounded-2xl p-4 space-y-2.5">
                <div className="flex items-start space-x-3">
                  <ShieldCheck className="w-5 h-5 text-blue-600 dark:text-blue-400 shrink-0 mt-0.5" aria-hidden="true" />
                  <div>
                    <h4 className="text-sm font-semibold text-blue-900 dark:text-blue-200">
                      Recommended Next Step
                    </h4>
                    <p className="text-xs text-blue-800 dark:text-blue-300/90 mt-0.5 leading-relaxed">
                      Configure default finishing options now (such as <strong>Duplex</strong>, <strong>Paper Size</strong>, and <strong>Color Model</strong>) so they apply to all print jobs.
                    </p>
                  </div>
                </div>

                <div className="pt-2 flex flex-col sm:flex-row gap-2">
                  <button
                    type="button"
                    onClick={handleOpenConfigForCreated}
                    className="flex-1 inline-flex items-center justify-center space-x-2 px-4 py-2.5 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-700 text-white shadow-sm transition cursor-pointer"
                  >
                    <Sliders className="w-4 h-4" aria-hidden="true" />
                    <span>Configure Default Options Now</span>
                    <ArrowRight className="w-4 h-4 ml-1" aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    onClick={onClose}
                    className="px-4 py-2.5 rounded-xl text-xs font-semibold bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-750 transition cursor-pointer"
                  >
                    Done / Close
                  </button>
                </div>
              </div>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-6">
              
              {/* Error banner */}
              {error && (
                <div
                  role="alert"
                  className="p-4 rounded-xl bg-rose-50 dark:bg-rose-950/50 border border-rose-200 dark:border-rose-900/60 flex items-start space-x-3"
                >
                  <AlertCircle className="w-5 h-5 text-rose-600 dark:text-rose-400 shrink-0 mt-0.5" aria-hidden="true" />
                  <p className="text-xs text-rose-800 dark:text-rose-300 font-medium leading-relaxed">
                    {error}
                  </p>
                </div>
              )}

              {/* SECTION 1: Connection Protocol */}
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <h3
                    id="add-printer-protocol-heading"
                    className="text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400 flex items-center space-x-1.5"
                  >
                    <Network className="w-3.5 h-3.5" aria-hidden="true" />
                    <span>1. Connection Protocol</span>
                  </h3>
                  <span className="text-xs font-semibold text-emerald-600 dark:text-emerald-400 flex items-center space-x-1">
                    <ShieldCheck className="w-3.5 h-3.5" aria-hidden="true" />
                    <span>AppSocket Recommended</span>
                  </span>
                </div>

                {/* Protocol Tabs */}
                <div
                  role="group"
                  aria-labelledby="add-printer-protocol-heading"
                  className="grid grid-cols-2 sm:grid-cols-3 gap-2"
                >
                  <button
                    type="button"
                    aria-pressed={protocol === 'socket'}
                    onClick={() => {
                      setProtocol('socket');
                      setPort('9100');
                    }}
                    className={`p-3 rounded-xl border text-left transition cursor-pointer relative ${
                      protocol === 'socket'
                        ? 'border-blue-600 bg-blue-50/70 dark:bg-blue-950/40 text-blue-900 dark:text-blue-100 ring-2 ring-blue-600/20'
                        : 'border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/40 text-slate-700 dark:text-slate-300 hover:border-slate-300'
                    }`}
                  >
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-xs font-bold">AppSocket / JetDirect</span>
                      <span className="px-1.5 py-0.2 rounded text-xs font-bold bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300">
                        Default
                      </span>
                    </div>
                    <p className="text-xs text-slate-500 dark:text-slate-400 leading-tight">
                      socket:// (Port 9100). Recommended standard protocol.
                    </p>
                  </button>

                  <button
                    type="button"
                    aria-pressed={protocol === 'ipp'}
                    onClick={() => {
                      setProtocol('ipp');
                      setPort('631');
                      setQueuePath('ipp/print');
                    }}
                    className={`p-3 rounded-xl border text-left transition cursor-pointer ${
                      protocol === 'ipp'
                        ? 'border-blue-600 bg-blue-50/70 dark:bg-blue-950/40 text-blue-900 dark:text-blue-100 ring-2 ring-blue-600/20'
                        : 'border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/40 text-slate-700 dark:text-slate-300 hover:border-slate-300'
                    }`}
                  >
                    <span className="text-xs font-bold block mb-1">IPP (Port 631)</span>
                    <p className="text-xs text-slate-500 dark:text-slate-400 leading-tight">
                      Internet Printing Protocol standard.
                    </p>
                  </button>

                  <button
                    type="button"
                    aria-pressed={protocol === 'ipps'}
                    onClick={() => {
                      setProtocol('ipps');
                      setPort('443');
                      setQueuePath('ipp/print');
                    }}
                    className={`p-3 rounded-xl border text-left transition cursor-pointer ${
                      protocol === 'ipps'
                        ? 'border-blue-600 bg-blue-50/70 dark:bg-blue-950/40 text-blue-900 dark:text-blue-100 ring-2 ring-blue-600/20'
                        : 'border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/40 text-slate-700 dark:text-slate-300 hover:border-slate-300'
                    }`}
                  >
                    <span className="text-xs font-bold block mb-1">IPPS (TLS/SSL)</span>
                    <p className="text-xs text-slate-500 dark:text-slate-400 leading-tight">
                      Encrypted TLS IPP delivery.
                    </p>
                  </button>

                  <button
                    type="button"
                    aria-pressed={protocol === 'lpd'}
                    onClick={() => {
                      setProtocol('lpd');
                      setPort('515');
                      setQueuePath('raw');
                    }}
                    className={`p-3 rounded-xl border text-left transition cursor-pointer ${
                      protocol === 'lpd'
                        ? 'border-blue-600 bg-blue-50/70 dark:bg-blue-950/40 text-blue-900 dark:text-blue-100 ring-2 ring-blue-600/20'
                        : 'border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/40 text-slate-700 dark:text-slate-300 hover:border-slate-300'
                    }`}
                  >
                    <span className="text-xs font-bold block mb-1">LPD / LPR</span>
                    <p className="text-xs text-slate-500 dark:text-slate-400 leading-tight">
                      Line Printer Daemon (Port 515).
                    </p>
                  </button>

                  <button
                    type="button"
                    aria-pressed={protocol === 'everywhere'}
                    onClick={() => {
                      setProtocol('everywhere');
                    }}
                    className={`p-3 rounded-xl border text-left transition cursor-pointer ${
                      protocol === 'everywhere'
                        ? 'border-blue-600 bg-blue-50/70 dark:bg-blue-950/40 text-blue-900 dark:text-blue-100 ring-2 ring-blue-600/20'
                        : 'border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/40 text-slate-700 dark:text-slate-300 hover:border-slate-300'
                    }`}
                  >
                    <span className="text-xs font-bold block mb-1">AirPrint / Everywhere</span>
                    <p className="text-xs text-slate-500 dark:text-slate-400 leading-tight">
                      Driverless IPP Everywhere.
                    </p>
                  </button>

                  <button
                    type="button"
                    aria-pressed={protocol === 'custom'}
                    onClick={() => setProtocol('custom')}
                    className={`p-3 rounded-xl border text-left transition cursor-pointer ${
                      protocol === 'custom'
                        ? 'border-blue-600 bg-blue-50/70 dark:bg-blue-950/40 text-blue-900 dark:text-blue-100 ring-2 ring-blue-600/20'
                        : 'border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/40 text-slate-700 dark:text-slate-300 hover:border-slate-300'
                    }`}
                  >
                    <span className="text-xs font-bold block mb-1">Custom URI</span>
                    <p className="text-xs text-slate-500 dark:text-slate-400 leading-tight">
                      Raw connection string.
                    </p>
                  </button>
                </div>

                {/* Driverless IPP Everywhere Notice */}
                {protocol === 'everywhere' && (
                  <div className="p-3 rounded-xl bg-amber-50 dark:bg-amber-950/50 border border-amber-200 dark:border-amber-900/60 flex items-start space-x-2.5">
                    <AlertTriangle className="w-4 h-4 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" aria-hidden="true" />
                    <p className="text-xs text-amber-800 dark:text-amber-300 leading-relaxed">
                      <strong>Notice:</strong> Driverless queues query capabilities dynamically over the network, which may restrict advanced finishing options. AppSocket/HP JetDirect is recommended for fixed queues.
                    </p>
                  </div>
                )}

                {/* Connection Parameters Inputs */}
                {protocol !== 'custom' ? (
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-1">
                    <div className="sm:col-span-2">
                      <label
                        htmlFor="add-printer-host"
                        className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                      >
                        Printer IP Address or Hostname <span className="text-rose-500">*</span>
                      </label>
                      <input
                        id="add-printer-host"
                        type="text"
                        value={host}
                        onChange={(e) => setHost(e.target.value)}
                        placeholder="e.g. 192.168.1.150 or printer.corp.internal"
                        className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                        required
                      />
                    </div>

                    <div>
                      <label
                        htmlFor="add-printer-port"
                        className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                      >
                        Port
                      </label>
                      <input
                        id="add-printer-port"
                        type="text"
                        value={port}
                        onChange={(e) => setPort(e.target.value)}
                        className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                      />
                    </div>
                  </div>
                ) : (
                  <div className="pt-1">
                    <label
                      htmlFor="add-printer-custom-uri"
                      className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                    >
                      Full Device URI <span className="text-rose-500">*</span>
                    </label>
                    <input
                      id="add-printer-custom-uri"
                      aria-describedby="add-printer-custom-uri-hint"
                      type="text"
                      value={customURI}
                      onChange={(e) => setCustomURI(e.target.value)}
                      placeholder="e.g. socket://192.168.1.50:9100 or usb://HP/LaserJet"
                      className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                      required
                    />
                    <p id="add-printer-custom-uri-hint" className="mt-1 text-xs text-slate-500 dark:text-slate-400">
                      For a virtual (null) queue that discards jobs, enter{' '}
                      <code className="font-mono">file:/dev/null</code>.
                    </p>
                  </div>
                )}

                {/* Connection URI Preview */}
                <div className="px-3.5 py-2 rounded-xl bg-slate-100 dark:bg-slate-800/80 border border-slate-200 dark:border-slate-700 flex items-center justify-between text-xs">
                  <span className="text-slate-500 dark:text-slate-400 font-mono text-xs">Device URI:</span>
                  <span className="font-mono font-semibold text-blue-600 dark:text-blue-400 truncate max-w-sm">
                    {computeURI() || '(enter host to preview)'}
                  </span>
                </div>
              </div>

              {/* SECTION 2: Queue Identification */}
              <div className="space-y-3 pt-2 border-t border-slate-100 dark:border-slate-800">
                <div className="flex items-center justify-between">
                  <h3 className="text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400 flex items-center space-x-1.5">
                    <FileText className="w-3.5 h-3.5" aria-hidden="true" />
                    <span>2. Queue Identification</span>
                  </h3>

                  {/* Safe Naming Toggle */}
                  <label className="inline-flex items-center space-x-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={safeNaming}
                      onChange={(e) => {
                        const checked = e.target.checked;
                        setSafeNaming(checked);
                        if (checked) {
                          const sanitized = name.replace(/[^a-zA-Z0-9]/g, '');
                          setName(sanitized);
                          setDescription(sanitized);
                        }
                      }}
                      className="rounded border-slate-300 text-blue-600 focus:ring-blue-500 w-3.5 h-3.5 cursor-pointer"
                    />
                    <span className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center space-x-1">
                      <ShieldCheck className="w-3.5 h-3.5 text-blue-600 dark:text-blue-400" aria-hidden="true" />
                      <span>Safe Queue Naming</span>
                    </span>
                  </label>
                </div>

                {safeNaming && (
                  <div className="px-3.5 py-2 rounded-xl bg-blue-50/70 dark:bg-blue-950/40 border border-blue-200 dark:border-blue-900/40 text-xs text-blue-800 dark:text-blue-300 leading-relaxed">
                    Enforces alphanumeric characters (A–Z, 0–9) and matches description to name to prevent whitespace mismatches.
                  </div>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label
                      htmlFor="add-printer-name"
                      className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                    >
                      Queue Name <span className="text-rose-500">*</span>
                    </label>
                    <input
                      id="add-printer-name"
                      aria-describedby="add-printer-name-hint"
                      type="text"
                      value={name}
                      onChange={(e) => handleNameChange(e.target.value)}
                      placeholder={safeNaming ? 'e.g. OfficeLaserJetPro' : 'e.g. Office_LaserJet_Pro'}
                      className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                      required
                      maxLength={127}
                    />
                    <p id="add-printer-name-hint" className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                      {safeNaming
                        ? 'Internal queue ID used by CUPS and PaperCut. Letters and numbers only (A–Z, 0–9).'
                        : 'Internal queue ID used by CUPS and PaperCut. Max 127 characters; spaces and / \\ \' " ? # are replaced with _.'}
                    </p>
                  </div>

                  <div>
                    <label
                      htmlFor="add-printer-description"
                      className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                    >
                      Queue Description {safeNaming && '(Synced with Name)'}
                    </label>
                    <input
                      id="add-printer-description"
                      aria-describedby="add-printer-description-hint"
                      type="text"
                      value={description}
                      disabled={safeNaming}
                      onChange={(e) => setDescription(e.target.value)}
                      placeholder="e.g. HP Color LaserJet 2nd Floor"
                      className={`w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 ${
                        safeNaming
                          ? 'bg-slate-100 dark:bg-slate-800/60 text-slate-500 cursor-not-allowed'
                          : 'bg-white dark:bg-slate-800'
                      }`}
                    />
                    <p id="add-printer-description-hint" className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                      Shown to users in the macOS print dialog. Spaces and special characters are allowed.
                    </p>
                  </div>
                </div>

                {name.trim() && (
                  <div className="px-3.5 py-2.5 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 text-xs text-slate-600 dark:text-slate-300 space-y-1">
                    <div>
                      <span className="font-semibold text-slate-700 dark:text-slate-200">Users will see: </span>
                      <span className="text-slate-900 dark:text-white">
                        {safeNaming ? name.trim() : description.trim() || name.trim()}
                      </span>
                    </div>
                    <div>
                      <span className="font-semibold text-slate-700 dark:text-slate-200">Queue ID: </span>
                      <code className="font-mono text-slate-900 dark:text-white">{name.trim()}</code>
                    </div>
                  </div>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
                  <div>
                    <label
                      htmlFor="add-printer-location"
                      className="block text-xs font-semibold text-slate-700 dark:text-slate-300 mb-1"
                    >
                      Location
                    </label>
                    <input
                      id="add-printer-location"
                      type="text"
                      value={location}
                      onChange={(e) => setLocation(e.target.value)}
                      placeholder="e.g. Building B - Level 2"
                      className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                  </div>

                  {/* Sharing Checkbox - Unshared by Default */}
                  <div className="flex items-center pt-5">
                    <label className="inline-flex items-start space-x-2.5 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={shared}
                        onChange={(e) => setShared(e.target.checked)}
                        className="rounded border-slate-300 text-blue-600 focus:ring-blue-500 w-4 h-4 mt-0.5 cursor-pointer"
                      />
                      <div>
                        <span className="text-xs font-semibold text-slate-800 dark:text-slate-200">
                          Share This Printer Queue
                        </span>
                        <p className="text-xs text-slate-500 dark:text-slate-400 leading-tight">
                          Default is unchecked to prevent unintended mDNS broadcasts.
                        </p>
                      </div>
                    </label>
                  </div>
                </div>
              </div>

              {/* SECTION 3: Driver (PPD) Selection */}
              <div className="space-y-3 pt-2 border-t border-slate-100 dark:border-slate-800">
                <div className="flex items-center justify-between">
                  <h3 className="text-xs font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400 flex items-center space-x-1.5">
                    <Sliders className="w-3.5 h-3.5" aria-hidden="true" />
                    <span>3. Driver (PPD) Selection</span>
                  </h3>

                  <div
                    role="tablist"
                    aria-label="Driver selection mode"
                    className="inline-flex p-0.5 rounded-lg bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-700"
                  >
                    <button
                      type="button"
                      role="tab"
                      aria-selected={driverMode === 'catalog'}
                      onClick={() => setDriverMode('catalog')}
                      className={`px-2.5 py-1 rounded-md text-xs font-semibold transition cursor-pointer ${
                        driverMode === 'catalog'
                          ? 'bg-white dark:bg-slate-700 text-blue-600 dark:text-blue-400 shadow-xs'
                          : 'text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200'
                      }`}
                    >
                      Driver Catalog
                    </button>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={driverMode === 'upload'}
                      onClick={() => setDriverMode('upload')}
                      className={`px-2.5 py-1 rounded-md text-xs font-semibold transition cursor-pointer ${
                        driverMode === 'upload'
                          ? 'bg-white dark:bg-slate-700 text-blue-600 dark:text-blue-400 shadow-xs'
                          : 'text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200'
                      }`}
                    >
                      Upload Custom PPD
                    </button>
                  </div>
                </div>

                {driverMode === 'catalog' ? (
                  <div className="space-y-2">
                    {/* Driver Search */}
                    <div className="relative">
                      <label htmlFor="add-printer-driver-search" className="sr-only">
                        Search drivers by make, model, or file name
                      </label>
                      <Search className="w-4 h-4 text-slate-500 dark:text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" aria-hidden="true" />
                      <input
                        id="add-printer-driver-search"
                        type="text"
                        value={driverSearch}
                        onChange={(e) => {
                          setDriverSearch(e.target.value);
                          loadDrivers(e.target.value);
                        }}
                        placeholder="Search drivers by make, model, or file name..."
                        aria-label="Search drivers by make, model, or file name"
                        className="w-full pl-9 pr-4 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                      />
                    </div>

                    {/* Drivers List */}
                    <div className="max-h-48 overflow-y-auto rounded-xl border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
                      {loadingDrivers ? (
                        <div className="p-4 text-center text-xs text-slate-500 dark:text-slate-400 flex items-center justify-center space-x-2">
                          <Loader2 className="w-4 h-4 animate-spin text-blue-500" aria-hidden="true" />
                          <span>Searching drivers...</span>
                        </div>
                      ) : drivers.length === 0 ? (
                        <div className="p-4 text-center text-xs text-slate-500 dark:text-slate-400">
                          No drivers matched your search criteria.
                        </div>
                      ) : (
                        drivers.map((d) => {
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
                  </div>
                ) : (
                  /* Custom PPD Upload */
                  <div className="space-y-3">
                    <label
                      htmlFor={fileInputId}
                      className="border-2 border-dashed border-slate-300 dark:border-slate-700 hover:border-blue-500 rounded-2xl p-6 text-center flex flex-col items-center justify-center cursor-pointer transition bg-slate-50/50 dark:bg-slate-800/40"
                    >
                      <input
                        id={fileInputId}
                        type="file"
                        accept=".ppd,.ppd.gz,.gz"
                        onChange={handleFileChange}
                        className="sr-only"
                      />
                      <Upload className="w-8 h-8 text-blue-600 dark:text-blue-400 mb-2" aria-hidden="true" />
                      <p className="text-xs font-semibold text-slate-800 dark:text-slate-200">
                        {uploadedFile ? uploadedFile.name : 'Click to select custom PPD file (.ppd or .ppd.gz)'}
                      </p>
                      <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                        Pre-flight checked automatically against Adobe PPD Specification 4.3 using cupstestppd
                      </p>
                    </label>

                    {validatingPPD && (
                      <div className="p-3 text-center text-xs text-slate-500 dark:text-slate-400 flex items-center justify-center space-x-2">
                        <Loader2 className="w-4 h-4 animate-spin text-blue-500" aria-hidden="true" />
                        <span>Validating PPD syntax with cupstestppd...</span>
                      </div>
                    )}

                    {ppdValidation && (
                      <div
                        role={ppdValidation.valid ? 'status' : 'alert'}
                        className={`p-3.5 rounded-xl border text-xs space-y-1.5 ${
                          ppdValidation.valid
                            ? 'bg-emerald-50 dark:bg-emerald-950/40 border-emerald-200 dark:border-emerald-800 text-emerald-900 dark:text-emerald-200'
                            : 'bg-rose-50 dark:bg-rose-950/40 border-rose-200 dark:border-rose-800 text-rose-900 dark:text-rose-200'
                        }`}
                      >
                        <div className="flex items-center space-x-2 font-semibold">
                          {ppdValidation.valid ? (
                            <>
                              <CheckCircle2 className="w-4 h-4 text-emerald-600 dark:text-emerald-400" aria-hidden="true" />
                              <span>Valid PPD: {ppdValidation.model}</span>
                            </>
                          ) : (
                            <>
                              <AlertCircle className="w-4 h-4 text-rose-600 dark:text-rose-400" aria-hidden="true" />
                              <span>PPD Validation Failed</span>
                            </>
                          )}
                        </div>

                        {ppdValidation.errors && ppdValidation.errors.length > 0 && (
                          <div className="text-xs text-rose-700 dark:text-rose-300 font-mono space-y-1">
                            {ppdValidation.errors.map((e, idx) => (
                              <p key={idx}>• {e}</p>
                            ))}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )}
              </div>

              {/* Submit Buttons */}
              <div className="pt-4 flex items-center justify-end space-x-3 border-t border-slate-200 dark:border-slate-800">
                <button
                  type="button"
                  onClick={onClose}
                  className="px-4 py-2.5 rounded-xl text-xs font-semibold bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-750 transition cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting || !name || !computeURI()}
                  className="inline-flex items-center space-x-2 px-5 py-2.5 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-700 text-white shadow-sm transition disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
                >
                  {submitting ? (
                    <>
                      <Loader2 className="w-4 h-4 animate-spin" />
                      <span>Creating Queue...</span>
                    </>
                  ) : (
                    <>
                      <PrinterIcon className="w-4 h-4" />
                      <span>Create Print Queue</span>
                    </>
                  )}
                </button>
              </div>
            </form>
          )}

        </div>
      </div>
    </div>
  );
};
