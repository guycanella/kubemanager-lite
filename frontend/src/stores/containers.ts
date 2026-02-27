import { writable, derived } from 'svelte/store';

// ─── Types ────────────────────────────────────────────────────────────────────

/** Snapshot of a Docker container returned by `ListContainers`. */
export interface ContainerInfo {
  /** Full 64-character container ID. */
  id: string;
  /** First 12 characters of the container ID, for display purposes. */
  shortId: string;
  /** Human-readable container name (without leading `/`). */
  name: string;
  /** Image reference used to create the container (e.g. `nginx:latest`). */
  image: string;
  /** Verbose status string from Docker (e.g. `"Up 3 hours"`). */
  status: string;
  /** Simplified container state: `"running"`, `"exited"`, `"paused"`, etc. */
  state: string;
  /** Unix timestamp (seconds) when the container was created. */
  created: number;
  /** CPU usage as a percentage of one core (0–100+). Updated via `stats:update` events. */
  cpuPercent: number;
  /** Current memory consumption in megabytes. Updated via `stats:update` events. */
  memoryUsageMB: number;
  /** Memory limit configured for the container in megabytes (0 = unlimited). */
  memoryLimitMB: number;
}

/** Payload emitted by the backend on every `stats:update` event (~1 per second per container). */
export interface StatsUpdate {
  /** ID of the container this update belongs to. */
  containerId: string;
  /** CPU usage percentage at the time of the sample. */
  cpuPercent: number;
  /** Memory usage in megabytes at the time of the sample. */
  memoryUsageMB: number;
  /** Memory limit in megabytes (0 = unlimited). */
  memoryLimitMB: number;
}

/**
 * A single log line emitted by the Hub backpressure system.
 * Multiple `LogMessage` entries are delivered together as a `log:batch` event.
 */
export interface LogMessage {
  /** Origin of the log line: `"docker"` or `"kubernetes"`. */
  source: string;
  /** Container ID (Docker) or pod name (Kubernetes) that produced this line. */
  id: string;
  /** Human-readable container or pod name. */
  name: string;
  /** Raw log line content (may include ANSI escape codes). */
  line: string;
  /** Unix timestamp in milliseconds when the line was captured. */
  timestamp: number;
}

/** The two top-level navigation tabs in the application. */
export type AppTab = 'docker' | 'kubernetes';

/**
 * Lifecycle state of a backend connection (Docker daemon or Kubernetes cluster).
 * - `connected`   — the last health check succeeded.
 * - `reconnecting`— a failure was detected; exponential backoff is in progress.
 * - `failed`      — maximum retry attempts reached; manual intervention required.
 * - `unknown`     — initial state before the first health check completes.
 */
export type ConnectionState = 'connected' | 'reconnecting' | 'failed' | 'unknown';

/** Full reconnection status payload delivered via the `connection:status` Wails event. */
export interface ConnectionStatus {
  /** Current lifecycle state of the connection. */
  state: ConnectionState;
  /** Human-readable description of the current state (e.g. `"Retrying in 4s…"`). */
  message: string;
  /** Seconds until the next reconnection attempt (0 when not reconnecting). */
  retryIn: number;
  /** Number of consecutive failed connection attempts since the last success. */
  attempt: number;
}

// ─── Stores ───────────────────────────────────────────────────────────────────

/** All Docker containers returned by the last `ListContainers` call. */
export const containers = writable<ContainerInfo[]>([]);

/**
 * ID of the container or pod whose logs are currently displayed in the LogViewer.
 * `null` means the log panel is closed.
 */
export const activeLogContainerId = writable<string | null>(null);

/**
 * Log lines grouped by container/pod ID.
 * Each array is capped at 2,000 lines (oldest lines are dropped first).
 * Updated by `appendLogBatch` on every `log:batch` event.
 */
export const logLines = writable<Record<string, string[]>>({});

/** Currently active top-level tab. Defaults to `"docker"` on startup. */
export const activeTab = writable<AppTab>('docker');

/** `true` when the Docker daemon is reachable and responding to pings. */
export const dockerConnected = writable<boolean>(false);

/** `true` when the Kubernetes cluster (via `~/.kube/config`) is reachable. */
export const k8sConnected = writable<boolean>(false);

/**
 * Detailed reconnection status for the Docker daemon.
 * Populated by `connection:status` events with `source === "docker"`.
 */
export const dockerStatus = writable<ConnectionStatus>({ state: 'unknown', message: '', retryIn: 0, attempt: 0 });

/**
 * Detailed reconnection status for the Kubernetes cluster.
 * Populated by `connection:status` events with `source === "kubernetes"`.
 */
export const k8sStatus = writable<ConnectionStatus>({ state: 'unknown', message: '', retryIn: 0, attempt: 0 });

/** `true` while `ListContainers` is in flight; used to show a loading indicator. */
export const containersLoading = writable<boolean>(false);

// ─── Derived ──────────────────────────────────────────────────────────────────

/**
 * The full `ContainerInfo` object for the currently active log target.
 * Derived from `containers` and `activeLogContainerId`.
 * Returns `null` when no log panel is open or when the ID is not found.
 */
export const activeContainer = derived(
  [containers, activeLogContainerId],
  ([$containers, $activeId]) =>
    $activeId ? $containers.find(c => c.id === $activeId) ?? null : null
);

// ─── Toast ────────────────────────────────────────────────────────────────────

/** A transient notification shown in the toast stack. */
export interface Toast {
  /** Unique numeric ID assigned at creation time. Used to dismiss a specific toast. */
  id: number;
  /** Visual variant controlling the icon and colour of the toast. */
  type: 'success' | 'error' | 'warning' | 'info';
  /** Text content displayed inside the toast. */
  message: string;
  /** When `true`, the toast will not auto-dismiss after 4 seconds. */
  persistent: boolean;
}

let _nextToastId = 0;

/** Active toast notifications. Components subscribe to render the toast stack. */
export const toasts = writable<Toast[]>([]);

/**
 * Push a new toast notification onto the stack.
 * Non-persistent toasts are automatically dismissed after 4 seconds.
 *
 * @param message   Text to display inside the toast.
 * @param type      Visual variant — `'info'` (default), `'success'`, `'warning'`, or `'error'`.
 * @param persistent When `true`, the toast stays until `dismissToast` is called explicitly.
 */
export function addToast(message: string, type: Toast['type'] = 'info', persistent = false) {
  const id = _nextToastId++;
  toasts.update(all => [...all, { id, type, message, persistent }]);
  if (!persistent) {
    setTimeout(() => dismissToast(id), 4000);
  }
}

/**
 * Remove a specific toast from the stack by its ID.
 * Safe to call even if the toast has already been dismissed.
 *
 * @param id The numeric ID returned (implicitly) when the toast was created.
 */
export function dismissToast(id: number) {
  toasts.update(all => all.filter(t => t.id !== id));
}

/**
 * Merge a `stats:update` payload into the `containers` store.
 * Only the CPU and memory fields are updated; all other fields are preserved.
 * Called directly from the `stats:update` Wails event listener in `App.svelte`.
 *
 * @param update The stats payload received from the backend.
 */
export function applyStatsUpdate(update: StatsUpdate) {
  containers.update(list =>
    list.map(c =>
      c.id === update.containerId
        ? {
            ...c,
            cpuPercent: update.cpuPercent,
            memoryUsageMB: update.memoryUsageMB,
            memoryLimitMB: update.memoryLimitMB,
          }
        : c
    )
  );
}

/**
 * Append a batch of log lines to the `logLines` store.
 * Lines are grouped by `msg.id` and the per-container buffer is capped at 2,000 entries
 * (oldest lines are dropped when the limit is exceeded).
 * Called directly from the `log:batch` Wails event listener in `App.svelte`.
 *
 * @param batch Array of `LogMessage` objects delivered by the Hub aggregator.
 */
export function appendLogBatch(batch: LogMessage[]) {
  logLines.update(all => {
    const updated = { ...all };
    for (const msg of batch) {
      if (!updated[msg.id]) updated[msg.id] = [];
      updated[msg.id] = [...updated[msg.id], msg.line].slice(-2000); // keep last 2000 lines
    }
    return updated;
  });
}
