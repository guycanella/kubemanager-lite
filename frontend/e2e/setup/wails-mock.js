// Injected before every test page load.
// Provides a fallback window.go when running outside the Wails WebView
// (e.g. Playwright in CI connecting directly to the Vite dev server).
// When the real Wails runtime is present, it is already attached to window.go
// before this script runs, so the guard below is a no-op.
if (!window.go) {
    window.go = {
      main: {
        App: {
          DockerStatus:     () => Promise.resolve(true),
          K8sStatus:        () => Promise.resolve(true),
  
          ListContainers: () => Promise.resolve([
            {
              id:            'abc123def456789',
              shortId:       'abc123def456',
              name:          'kubemanager-test',
              image:         'alpine:latest',
              status:        'Up 2 minutes',
              state:         'running',
              created:       Math.floor(Date.now() / 1000) - 120,
              cpuPercent:    1.5,
              memoryUsageMB: 12.4,
              memoryLimitMB: 512.0,
            }
          ]),
  
          StartStatsStream: () => Promise.resolve(),
          StopStatsStream:  () => Promise.resolve(),
          StartLogStream:   () => Promise.resolve(),
          StopLogStream:    () => Promise.resolve(),
  
          ListNamespaces: () => Promise.resolve(['default', 'kube-system']),
  
          ListPods: () => Promise.resolve([
            {
              name:      'kubemanager-test',
              namespace: 'default',
              status:    'Running',
              ready:     true,
              restarts:  0,
              nodeName:  'ci-node',
              age:       Math.floor(Date.now() / 1000) - 300,
              image:     'alpine:latest',
            }
          ]),
  
          StartPodLogStream: () => Promise.resolve(),
          StopPodLogStream:  () => Promise.resolve(),
        }
      }
    };
  }
  
  // Wails runtime stubs (EventsOn, EventsOff, WindowToggleMaximise, etc.)
  if (!window.runtime) {
    window.runtime = {};
  }
  if (!window.WailsInvoke) {
    window.WailsInvoke = () => {};
  }