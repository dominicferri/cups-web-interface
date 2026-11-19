import type { AccessoryOption, AccessoryReport } from '../types/cups';

// Choice names drivers use for "this accessory is not fitted". Display only:
// it decides which rows are listed first and what counts as installed, never
// what is saved.
const NOT_FITTED = new Set(['false', 'none', 'notinstalled', 'not installed', 'no', 'off', 'uninstalled']);

export function choiceLabel(opt: AccessoryOption, value: string): string {
  return opt.choices.find((c) => c.name === value)?.label || value;
}

export function isFitted(opt: AccessoryOption, value = opt.value): boolean {
  const label = choiceLabel(opt, value).toLowerCase();
  return !NOT_FITTED.has(value.toLowerCase()) && !NOT_FITTED.has(label) && !label.startsWith('not ');
}

// Number of accessories the queue has marked as installed, or null when the
// queue has no installable options.
export function installedCount(report: AccessoryReport | null): number | null {
  if (!report || !report.available) return null;
  return report.options.filter((o) => isFitted(o)).length;
}
