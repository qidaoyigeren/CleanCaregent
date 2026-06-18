/**
 * Performance monitoring utility.
 * Collects Core Web Vitals, API request timing, and custom app metrics.
 * All data is logged to console in dev; in production, send to an analytics endpoint.
 */

// ===== Types =====

export interface WebVital {
  name: string;
  value: number;
  rating: 'good' | 'needs-improvement' | 'poor';
  delta: number;
  id: string;
}

export interface ApiMetric {
  path: string;
  method: string;
  status: number;
  duration: number;
  cached: boolean;
  timestamp: number;
}

export interface CustomMetric {
  name: string;
  value: number;
  tags?: Record<string, string>;
  timestamp: number;
}

export interface PerfSummary {
  webVitals: WebVital[];
  apiCalls: ApiMetric[];
  customMetrics: CustomMetric[];
  sessionDuration: number;
}

// ===== Storage =====

const vitals: WebVital[] = [];
const apiCalls: ApiMetric[] = [];
const customMetrics: CustomMetric[] = [];
const sessionStart = Date.now();

// Config
const MAX_ENTRIES = 200; // Per category
const REPORT_INTERVAL = 60_000; // Auto-report every 60s in production
let reportEndpoint: string | null = null;
let reportTimer: ReturnType<typeof setInterval> | null = null;

// ===== Web Vitals =====

/** Thresholds for Core Web Vitals (in ms or unitless for CLS) */
const THRESHOLDS = {
  LCP: { good: 2500, poor: 4000 },
  FID: { good: 100, poor: 300 },
  CLS: { good: 0.1, poor: 0.25 },
  FCP: { good: 1800, poor: 3000 },
  TTFB: { good: 800, poor: 1800 },
  INP: { good: 200, poor: 500 },
};

function rateMetric(name: string, value: number): WebVital['rating'] {
  const threshold = THRESHOLDS[name as keyof typeof THRESHOLDS];
  if (!threshold) return 'good';
  if (value <= threshold.good) return 'good';
  if (value <= threshold.poor) return 'needs-improvement';
  return 'poor';
}

/**
 * Observe a PerformanceEntry and record it as a Web Vital.
 */
function observeWebVital(entry: PerformanceEntry): void {
  let value: number;

  if (entry.entryType === 'largest-contentful-paint') {
    value = entry.startTime;
  } else if (entry.entryType === 'first-input') {
    value = (entry as PerformanceEventTiming).processingStart - entry.startTime;
  } else if (entry.entryType === 'layout-shift') {
    // CLS: only count if not caused by user input
    if ((entry as any).hadRecentInput) return;
    value = (entry as any).value;
  } else if (entry.entryType === 'paint') {
    value = entry.startTime;
  } else if (entry.entryType === 'navigation') {
    const nav = entry as PerformanceNavigationTiming;
    value = nav.responseStart - nav.requestStart; // TTFB
  } else {
    return;
  }

  const name = entry.entryType === 'layout-shift' ? 'CLS'
    : entry.entryType === 'largest-contentful-paint' ? 'LCP'
    : entry.entryType === 'first-input' ? 'FID'
    : entry.name === 'first-contentful-paint' ? 'FCP'
    : entry.entryType === 'navigation' ? 'TTFB'
    : entry.name;

  const vital: WebVital = {
    name,
    value: Math.round(value * 100) / 100,
    rating: rateMetric(name, value),
    delta: value,
    id: `${name}-${Date.now()}`,
  };

  vitals.push(vital);
  trimArray(vitals);

  if (import.meta.env.DEV) {
    const icon = vital.rating === 'good' ? '✅' : vital.rating === 'needs-improvement' ? '⚠️' : '❌';
    console.log(`[Perf] ${icon} ${name}: ${vital.value}${name === 'CLS' ? '' : 'ms'} (${vital.rating})`);
  }
}

/**
 * Initialize Web Vitals observers.
 */
function initWebVitals(): void {
  if (typeof PerformanceObserver === 'undefined') return;

  // LCP
  try {
    const lcp = new PerformanceObserver((list) => {
      const entries = list.getEntries();
      // LCP reports multiple entries; only the last is the final one
      entries.forEach(observeWebVital);
    });
    lcp.observe({ type: 'largest-contentful-paint', buffered: true });
  } catch { /* not supported */ }

  // FID
  try {
    const fid = new PerformanceObserver((list) => {
      list.getEntries().forEach(observeWebVital);
    });
    fid.observe({ type: 'first-input', buffered: true });
  } catch { /* not supported */ }

  // CLS
  try {
    const cls = new PerformanceObserver((list) => {
      list.getEntries().forEach(observeWebVital);
    });
    cls.observe({ type: 'layout-shift', buffered: true });
  } catch { /* not supported */ }

  // Paint (FCP)
  try {
    const paint = new PerformanceObserver((list) => {
      list.getEntries().forEach(observeWebVital);
    });
    paint.observe({ type: 'paint', buffered: true });
  } catch { /* not supported */ }

  // Navigation (TTFB)
  try {
    const nav = new PerformanceObserver((list) => {
      list.getEntries().forEach(observeWebVital);
    });
    nav.observe({ type: 'navigation', buffered: true });
  } catch { /* not supported */ }
}

// ===== API Metrics =====

/**
 * Record an API call metric.
 * Called by the API client after each request.
 */
export function recordApiMetric(metric: Omit<ApiMetric, 'timestamp'>): void {
  const entry: ApiMetric = { ...metric, timestamp: Date.now() };
  apiCalls.push(entry);
  trimArray(apiCalls);

  if (import.meta.env.DEV) {
    const icon = metric.duration < 500 ? '✅' : metric.duration < 2000 ? '⚠️' : '🐌';
    console.log(
      `[Perf] ${icon} API ${metric.method} ${metric.path}: ${Math.round(metric.duration)}ms [${metric.status}]${metric.cached ? ' (cached)' : ''}`
    );
  }
}

// ===== Custom Metrics =====

/**
 * Record a custom application metric.
 */
export function recordMetric(name: string, value: number, tags?: Record<string, string>): void {
  const entry: CustomMetric = { name, value, tags, timestamp: Date.now() };
  customMetrics.push(entry);
  trimArray(customMetrics);

  if (import.meta.env.DEV) {
    console.log(`[Perf] 📊 ${name}: ${Math.round(value)}ms`, tags || '');
  }
}

/**
 * Create a timer that returns elapsed ms when stopped.
 */
export function startTimer(): () => number {
  const start = performance.now();
  return () => performance.now() - start;
}

// ===== Reporting =====

/**
 * Get a summary of all collected metrics.
 */
export function getPerfSummary(): PerfSummary {
  return {
    webVitals: [...vitals],
    apiCalls: [...apiCalls],
    customMetrics: [...customMetrics],
    sessionDuration: Date.now() - sessionStart,
  };
}

/**
 * Send metrics to an analytics endpoint.
 */
async function reportMetrics(): Promise<void> {
  if (!reportEndpoint) return;

  const summary = getPerfSummary();
  // Only report if there's data
  if (summary.webVitals.length === 0 && summary.apiCalls.length === 0) return;

  try {
    await fetch(reportEndpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(summary),
      // Use keepalive to ensure the request completes even on page unload
      keepalive: true,
    });
  } catch {
    // Silently fail — don't let monitoring break the app
  }
}

// ===== Utilities =====

function trimArray<T>(arr: T[]): void {
  while (arr.length > MAX_ENTRIES) {
    arr.shift();
  }
}

// ===== Initialization =====

/**
 * Initialize the performance monitoring system.
 * Call once on app startup.
 *
 * @param options.reportEndpoint - URL to POST metrics to (production)
 * @param options.autoReport - Whether to auto-report on interval (default: true in production)
 */
export function initPerfMonitor(options?: {
  reportEndpoint?: string;
  autoReport?: boolean;
}): void {
  initWebVitals();

  if (options?.reportEndpoint) {
    reportEndpoint = options.reportEndpoint;
  }

  // Auto-report in production if endpoint is set
  const shouldAutoReport = options?.autoReport ?? (!import.meta.env.DEV && !!reportEndpoint);
  if (shouldAutoReport && !reportTimer) {
    reportTimer = setInterval(reportMetrics, REPORT_INTERVAL);
  }

  // Report on page unload
  if (typeof window !== 'undefined') {
    window.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'hidden') {
        reportMetrics();
      }
    });
  }

  if (import.meta.env.DEV) {
    console.log('[Perf] Performance monitoring initialized');
  }
}

/**
 * Manually trigger a metrics report.
 */
export { reportMetrics };
