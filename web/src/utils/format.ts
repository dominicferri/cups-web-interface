export function formatBytes(bytes: number, decimals = 1): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

export function formatAlert(alert: string): { label: string; severity: 'warning' | 'error' | 'info' } {
  switch (alert) {
    case 'media-empty-report':
    case 'media-empty':
    case 'paper-out':
      return { label: 'Paper Out / Tray Empty', severity: 'error' };
    case 'media-jam':
    case 'paper-jam':
      return { label: 'Paper Jam', severity: 'error' };
    case 'toner-low-warning':
    case 'low-toner':
      return { label: 'Low Toner / Ink', severity: 'warning' };
    case 'toner-empty':
      return { label: 'Toner Empty', severity: 'error' };
    case 'filter-failed':
      return { label: 'Driver Filter Failed', severity: 'error' };
    case 'door-open':
      return { label: 'Cover / Door Open', severity: 'warning' };
    case 'offline':
      return { label: 'Printer Offline', severity: 'error' };
    default:
      return { label: alert.replace(/-/g, ' '), severity: 'info' };
  }
}

// Puts drivers whose make appears in the queue's name or description first, so a
// queue named "Office_Xerox" opens on the Xerox PPDs instead of DYMO and Epson.
// The order within each group stays as lpinfo gave it.
export function driversForQueue<T extends { make: string }>(drivers: T[], ...labels: string[]): T[] {
  const text = labels.join(' ').toLowerCase();
  const matches = (d: T) => {
    const make = d.make.trim().toLowerCase();
    return make.length > 1 && make !== 'generic' && make !== 'raw' && text.includes(make);
  };
  return [...drivers.filter(matches), ...drivers.filter((d) => !matches(d))];
}
