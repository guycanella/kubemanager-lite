<script lang="ts">
    import { onMount, onDestroy } from 'svelte';
    import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
    import { dockerConnected, k8sConnected, dockerStatus, k8sStatus } from '../stores/containers';
  
    interface ConnectionEvent {
      source: string;
      state: 'connected' | 'reconnecting' | 'failed';
      message: string;
      retryIn: number;
      attempt: number;
    }
  
    // ─── Toast queue ─────────────────────────────────────────────────────────────
  
    interface Toast {
      id: number;
      type: 'success' | 'error' | 'warning' | 'info';
      message: string;
      persistent: boolean; // persistent toasts stay until dismissed
    }
  
    let toasts: Toast[] = [];
    let nextId = 0;
  
    export function addToast(message: string, type: Toast['type'] = 'info', persistent = false) {
      const id = nextId++;
      toasts = [...toasts, { id, type, message, persistent }];
  
      if (!persistent) {
        setTimeout(() => dismiss(id), 4000);
      }
    }
  
    function dismiss(id: number) {
      toasts = toasts.filter(t => t.id !== id);
    }
  
    // ─── Reconnect countdown ─────────────────────────────────────────────────────
  
    // Active reconnect toasts per source — keyed so we replace instead of stack
    let reconnectToastIds: Record<string, number> = {};
  
    function handleConnectionEvent(event: ConnectionEvent) {
      const source = event.source;
  
      if (event.state === 'connected') {
        // Remove the reconnecting toast if present
        if (reconnectToastIds[source] !== undefined) {
          dismiss(reconnectToastIds[source]);
          delete reconnectToastIds[source];
        }
  
        // Update stores
        if (source === 'Docker') {
          dockerConnected.set(true);
          dockerStatus.set({ state: 'connected', message: event.message, retryIn: 0, attempt: 0 });
        } else {
          k8sConnected.set(true);
          k8sStatus.set({ state: 'connected', message: event.message, retryIn: 0, attempt: 0 });
        }
  
        addToast(`${source} reconnected`, 'success');
        return;
      }
  
      if (event.state === 'reconnecting') {
        // Update stores
        if (source === 'Docker') {
          dockerConnected.set(false);
          dockerStatus.set({ state: 'reconnecting', message: event.message, retryIn: event.retryIn, attempt: event.attempt });
        } else {
          k8sConnected.set(false);
          k8sStatus.set({ state: 'reconnecting', message: event.message, retryIn: event.retryIn, attempt: event.attempt });
        }
  
        // Replace existing reconnect toast for this source
        if (reconnectToastIds[source] !== undefined) {
          dismiss(reconnectToastIds[source]);
        }
  
        const id = nextId++;
        reconnectToastIds[source] = id;
        toasts = [...toasts, {
          id,
          type: 'warning',
          message: event.message,
          persistent: true, // stays until connected or dismissed
        }];
      }
    }
  
    // ─── Lifecycle ───────────────────────────────────────────────────────────────
  
    onMount(() => {
      EventsOn('connection:status', handleConnectionEvent);
    });
  
    onDestroy(() => {
      EventsOff('connection:status');
    });
  
    // ─── Icon helpers ────────────────────────────────────────────────────────────
  
    const icons = {
      success: `<polyline points="20 6 9 17 4 12"/>`,
      error:   `<circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/>`,
      warning: `<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>`,
      info:    `<circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>`,
    };
  </script>
  
  <!-- ─── Toast container ──────────────────────────────────────────────────────── -->
  {#if toasts.length > 0}
    <div class="toast-container">
      {#each toasts as toast (toast.id)}
        <div class="toast {toast.type}" role="alert">
          <svg class="toast-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            {@html icons[toast.type]}
          </svg>
          <span class="toast-message">{toast.message}</span>
          <button class="toast-close" on:click={() => dismiss(toast.id)}>
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>
      {/each}
    </div>
  {/if}
  
  <style>
    .toast-container {
      position: fixed;
      bottom: 20px;
      right: 20px;
      display: flex;
      flex-direction: column;
      gap: 8px;
      z-index: 1000;
      pointer-events: none;
    }
  
    .toast {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 10px 14px;
      border-radius: 8px;
      border: 1px solid;
      backdrop-filter: blur(8px);
      font-size: 12px;
      font-weight: 500;
      max-width: 380px;
      pointer-events: all;
      animation: slideIn 0.2s ease;
    }
  
    @keyframes slideIn {
      from { opacity: 0; transform: translateX(16px); }
      to   { opacity: 1; transform: translateX(0); }
    }
  
    .toast.success {
      background: color-mix(in srgb, var(--green) 12%, #161B22);
      border-color: color-mix(in srgb, var(--green) 30%, transparent);
      color: var(--green);
    }
  
    .toast.error {
      background: color-mix(in srgb, var(--red) 12%, #161B22);
      border-color: color-mix(in srgb, var(--red) 30%, transparent);
      color: var(--red);
    }
  
    .toast.warning {
      background: color-mix(in srgb, var(--amber) 12%, #161B22);
      border-color: color-mix(in srgb, var(--amber) 30%, transparent);
      color: var(--amber);
    }
  
    .toast.info {
      background: color-mix(in srgb, var(--cyan) 12%, #161B22);
      border-color: color-mix(in srgb, var(--cyan) 30%, transparent);
      color: var(--cyan);
    }
  
    .toast-icon {
      flex-shrink: 0;
    }
  
    .toast-message {
      flex: 1;
      line-height: 1.4;
      color: var(--text);
    }
  
    .toast-close {
      flex-shrink: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      background: none;
      border: none;
      color: var(--text-muted);
      cursor: pointer;
      padding: 2px;
      border-radius: 3px;
      transition: color 0.15s;
    }
  
    .toast-close:hover {
      color: var(--text);
    }
  </style>