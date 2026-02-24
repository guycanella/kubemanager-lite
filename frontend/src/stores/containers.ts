import { writable, derived } from 'svelte/store';

// ─── Types ────────────────────────────────────────────────────────────────────

export interface ContainerInfo {
  id: string;
  shortId: string;
  name: string;
  image: string;
  status: string;
  state: string;
  created: number;
  cpuPercent: number;
  memoryUsageMB: number;
  memoryLimitMB: number;
}

export interface StatsUpdate {
  containerId: string;
  cpuPercent: number;
  memoryUsageMB: number;
  memoryLimitMB: number;
}

export interface LogMessage {
  source: string;
  id: string;
  name: string;
  line: string;
  timestamp: number;
}

export type AppTab = 'docker' | 'kubernetes';

export type ConnectionState = 'connected' | 'reconnecting' | 'failed' | 'unknown';

export interface ConnectionStatus {
  state: ConnectionState;
  message: string;
  retryIn: number;
  attempt: number;
}

// ─── Stores ───────────────────────────────────────────────────────────────────

// List of all running containers
export const containers = writable<ContainerInfo[]>([]);

// ID of the container whose logs are currently open (null = log panel closed)
export const activeLogContainerId = writable<string | null>(null);

// Log lines per container — keyed by container ID
export const logLines = writable<Record<string, string[]>>({});

// Active tab
export const activeTab = writable<AppTab>('docker');

// Connection status
export const dockerConnected = writable<boolean>(false);
export const k8sConnected = writable<boolean>(false);

// Detailed reconnection status per source
export const dockerStatus = writable<ConnectionStatus>({ state: 'unknown', message: '', retryIn: 0, attempt: 0 });
export const k8sStatus = writable<ConnectionStatus>({ state: 'unknown', message: '', retryIn: 0, attempt: 0 });

// Loading state for the container list
export const containersLoading = writable<boolean>(false);

// ─── Derived ──────────────────────────────────────────────────────────────────

// Currently selected container object
export const activeContainer = derived(
  [containers, activeLogContainerId],
  ([$containers, $activeId]) =>
    $activeId ? $containers.find(c => c.id === $activeId) ?? null : null
);

// ─── Helpers ──────────────────────────────────────────────────────────────────

// Apply a StatsUpdate to the containers store — called on every "stats:update" event
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

// Append a batch of log lines to the logLines store
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