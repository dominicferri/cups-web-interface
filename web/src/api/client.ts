import type {
  Printer,
  Job,
  SystemStatus,
  PrinterOptionsResponse,
  AccessoryReport,
  AccessorySuggestions,
  ArchAuditResult,
  FleetAuditSummary,
  AddPrinterParams,
  ModifyPrinterParams,
  DriverInfo,
  PPDValidationResult,
  ServerSettings,
  LogEntry,
  LaunchAgentStatus,
} from '../types/cups';

const API_BASE = '/api/v1';

// Session token management: reads token from URL query or sessionStorage, then cleans URL
export function getSessionToken(): string {
  if (typeof window === 'undefined') return '';
  const params = new URLSearchParams(window.location.search);
  const queryToken = params.get('token');
  if (queryToken) {
    sessionStorage.setItem('cups_webui_token', queryToken);
    // Remove token from browser address bar to avoid leaking in history or screen shares
    const cleanUrl = window.location.pathname + (window.location.hash || '');
    window.history.replaceState({}, document.title, cleanUrl);
    return queryToken;
  }
  return sessionStorage.getItem('cups_webui_token') || '';
}

let sessionInitPromise: Promise<string | null> | null = null;

// ensureSession acquires or refreshes the session cookie & token for same-origin browser clients
export async function ensureSession(): Promise<string | null> {
  if (typeof window === 'undefined') return null;
  const currentToken = getSessionToken();
  if (currentToken) return currentToken;

  if (sessionInitPromise) return sessionInitPromise;

  sessionInitPromise = (async () => {
    try {
      const res = await fetch(`${API_BASE}/auth/session`, {
        method: 'POST',
        headers: { 'X-Requested-With': 'CUPS-WebUI' },
        credentials: 'same-origin',
      });
      if (res.ok) {
        const data = await res.json();
        if (data.token) {
          sessionStorage.setItem('cups_webui_token', data.token);
          return data.token;
        }
      }
    } catch {
      // ignore
    } finally {
      sessionInitPromise = null;
    }
    return null;
  })();

  return sessionInitPromise;
}

// Automatically ensure session on load
if (typeof window !== 'undefined') {
  ensureSession();
}

async function apiFetch(input: string, init: RequestInit = {}): Promise<Response> {
  let token = getSessionToken();
  const headers = new Headers(init.headers || {});

  if (token && !headers.has('X-CUPS-WebUI-Token')) {
    headers.set('X-CUPS-WebUI-Token', token);
  }
  if (!headers.has('X-Requested-With')) {
    headers.set('X-Requested-With', 'CUPS-WebUI');
  }

  let res = await fetch(input, {
    ...init,
    headers,
    credentials: 'same-origin',
  });

  // If unauthorized on mutating request, attempt to establish/refresh session once and retry
  if (res.status === 401 && !input.includes('/auth/session')) {
    // Drop any stale token (e.g. the service restarted while this tab stayed open)
    // so ensureSession() fetches the current one instead of returning the old one.
    try {
      sessionStorage.removeItem('cups_webui_token');
    } catch {
      // ignore
    }
    const refreshedToken = await ensureSession();
    if (refreshedToken) {
      headers.set('X-CUPS-WebUI-Token', refreshedToken);
      res = await fetch(input, {
        ...init,
        headers,
        credentials: 'same-origin',
      });
    }
  }

  return res;
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let errMessage = `HTTP ${res.status}: ${res.statusText}`;
    try {
      const body = await res.json();
      if (body.error) {
        errMessage = body.error;
      }
    } catch {
      // ignore json parse error
    }
    if (res.status === 401) {
      errMessage = "Session expired. Reload the page and try again.";
    }
    throw new Error(errMessage);
  }
  return res.json();
}

export const api = {
  async getAuthStatus(): Promise<{ authenticated: boolean }> {
    const res = await apiFetch(`${API_BASE}/auth/status`);
    return handleResponse<{ authenticated: boolean }>(res);
  },

  async getSystemStatus(): Promise<SystemStatus> {
    const res = await apiFetch(`${API_BASE}/system/status`);
    return handleResponse<SystemStatus>(res);
  },

  async getPrinters(): Promise<Printer[]> {
    const res = await apiFetch(`${API_BASE}/printers`);
    return handleResponse<Printer[]>(res);
  },

  async getJobs(which: 'active' | 'completed' | 'all' = 'active', printer?: string): Promise<Job[]> {
    const params = new URLSearchParams();
    if (which) params.set('which', which);
    if (printer) params.set('printer', printer);
    const res = await apiFetch(`${API_BASE}/jobs?${params.toString()}`);
    return handleResponse<Job[]>(res);
  },

  async pausePrinter(name: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/pause`, {
      method: 'POST',
    });
    return handleResponse<void>(res);
  },

  async resumePrinter(name: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/resume`, {
      method: 'POST',
    });
    return handleResponse<void>(res);
  },

  async setDefaultPrinter(name: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/default`, {
      method: 'POST',
    });
    return handleResponse<void>(res);
  },

  async cancelJob(id: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/jobs/${encodeURIComponent(id)}/cancel`, {
      method: 'POST',
    });
    return handleResponse<void>(res);
  },

  async holdJob(id: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/jobs/${encodeURIComponent(id)}/hold`, {
      method: 'POST',
    });
    return handleResponse<void>(res);
  },

  async releaseJob(id: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/jobs/${encodeURIComponent(id)}/release`, {
      method: 'POST',
    });
    return handleResponse<void>(res);
  },

  async restartJob(id: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/jobs/${encodeURIComponent(id)}/restart`, {
      method: 'POST',
    });
    return handleResponse<void>(res);
  },

  async purgePrinter(name: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/purge`, {
      method: 'POST',
    });
    return handleResponse<void>(res);
  },

  async deletePrinter(name: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    });
    return handleResponse<void>(res);
  },

  async getPrinterOptions(name: string): Promise<PrinterOptionsResponse> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/options`);
    return handleResponse<PrinterOptionsResponse>(res);
  },

  async setPrinterOptions(name: string, options: Record<string, string>): Promise<void> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/options`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ options }),
    });
    return handleResponse<void>(res);
  },

  async getAccessories(name: string): Promise<AccessoryReport> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/accessories`);
    return handleResponse<AccessoryReport>(res);
  },

  // acknowledgedWarning must be true: the UI showed the "can't be checked" warning.
  async setAccessories(
    name: string,
    values: Record<string, string>,
    acknowledgedWarning: boolean,
  ): Promise<AccessoryReport> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/accessories`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ values, acknowledged_warning: acknowledgedWarning }),
    });
    return handleResponse<AccessoryReport>(res);
  },

  async resetAccessories(name: string): Promise<AccessoryReport> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/accessories/reset`, {
      method: 'POST',
    });
    return handleResponse<AccessoryReport>(res);
  },

  async suggestAccessories(name: string): Promise<AccessorySuggestions> {
    // POST: the server contacts the device, so this needs the session token.
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/accessories/suggestions`, {
      method: 'POST',
    });
    return handleResponse<AccessorySuggestions>(res);
  },

  async getPrinterAudit(name: string): Promise<ArchAuditResult> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/audit`);
    return handleResponse<ArchAuditResult>(res);
  },

  async getFleetAudit(): Promise<FleetAuditSummary> {
    const res = await apiFetch(`${API_BASE}/audit/summary`);
    return handleResponse<FleetAuditSummary>(res);
  },

  async addPrinter(params: AddPrinterParams): Promise<{ status: string; message: string; name: string }> {
    const res = await apiFetch(`${API_BASE}/printers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    return handleResponse<{ status: string; message: string; name: string }>(res);
  },

  async modifyPrinter(name: string, params: ModifyPrinterParams): Promise<void> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    return handleResponse<void>(res);
  },

  async setPrinterDriver(name: string, driver: string): Promise<void> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/driver`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ driver }),
    });
    return handleResponse<void>(res);
  },

  async printTestPage(name: string): Promise<{ status: string; message: string; job_id: string }> {
    const res = await apiFetch(`${API_BASE}/printers/${encodeURIComponent(name)}/test-page`, {
      method: 'POST',
    });
    return handleResponse<{ status: string; message: string; job_id: string }>(res);
  },

  async getDrivers(search?: string): Promise<DriverInfo[]> {
    const params = new URLSearchParams();
    if (search) params.set('search', search);
    const res = await apiFetch(`${API_BASE}/drivers?${params.toString()}`);
    return handleResponse<DriverInfo[]>(res);
  },

  async validatePPD(file: File): Promise<PPDValidationResult> {
    const formData = new FormData();
    formData.append('ppd', file);
    const res = await apiFetch(`${API_BASE}/ppds/validate`, {
      method: 'POST',
      body: formData,
    });
    return handleResponse<PPDValidationResult>(res);
  },

  async getServerSettings(): Promise<ServerSettings> {
    const res = await apiFetch(`${API_BASE}/server/settings`);
    return handleResponse<ServerSettings>(res);
  },

  async setServerSettings(settings: ServerSettings): Promise<void> {
    const res = await apiFetch(`${API_BASE}/server/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(settings),
    });
    return handleResponse<void>(res);
  },

  async getLogs(lines: number = 100, level: string = 'all'): Promise<LogEntry[]> {
    const res = await apiFetch(`${API_BASE}/system/logs?lines=${lines}&level=${encodeURIComponent(level)}`);
    return handleResponse<LogEntry[]>(res);
  },

  async getLaunchAgentStatus(): Promise<LaunchAgentStatus> {
    const res = await apiFetch(`${API_BASE}/system/launchagent`);
    return handleResponse<LaunchAgentStatus>(res);
  },

  async manageLaunchAgent(action: 'install' | 'uninstall'): Promise<{ status: string; message: string }> {
    const res = await apiFetch(`${API_BASE}/system/launchagent`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action }),
    });
    return handleResponse<{ status: string; message: string }>(res);
  },

  subscribeEvents(onEvent: (event: { type: string; action?: string; target?: string }) => void): () => void {
    if (typeof window === 'undefined' || !window.EventSource) {
      return () => {};
    }

    let eventSource: EventSource | null = null;
    try {
      const token = getSessionToken();
      const url = token ? `${API_BASE}/events?token=${encodeURIComponent(token)}` : `${API_BASE}/events`;
      eventSource = new EventSource(url);
      eventSource.onmessage = (e) => {
        try {
          const parsed = JSON.parse(e.data);
          onEvent(parsed);
        } catch {
          // heartbeat or unparseable frame
        }
      };
      eventSource.onerror = () => {
        // EventSource will auto-reconnect
      };
    } catch {
      // Ignore initial setup errors
    }

    return () => {
      if (eventSource) {
        eventSource.close();
      }
    };
  },
};

export const apiClient = api;
