import { test as base } from '@playwright/test';

export const test = base.extend({
  page: async ({ page }, use) => {
    await page.addInitScript(() => {
      if (!(window as any).go) {
        (window as any).go = {
          main: {
            App: {
              DockerStatus:      () => Promise.resolve(true),
              K8sStatus:         () => Promise.resolve(true),
              ListContainers:    () => Promise.resolve([
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
              StartStatsStream:  () => Promise.resolve(),
              StopStatsStream:   () => Promise.resolve(),
              StartLogStream:    () => Promise.resolve(),
              StopLogStream:     () => Promise.resolve(),
              ListNamespaces:    () => Promise.resolve(['default', 'kube-system']),
              ListPods:          () => Promise.resolve([
                {
                  name:      'kubemanager-test-pod',
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
      if (!(window as any).runtime) {
        (window as any).runtime = {};
      }
    });
    await use(page);
  },
});

export { expect } from '@playwright/test';