import { setup } from 'xstate'
import type { SidecarStatus } from '@/shared/desktop/desktopBridge'

export const sidecarMachine = setup({
  types: {
    context: {} as { lastStatus: SidecarStatus | null },
    events: {} as
      | { type: 'CHECK' }
      | { type: 'ONLINE'; status: SidecarStatus }
      | { type: 'OFFLINE'; status: SidecarStatus }
      | { type: 'RESTART' },
  },
}).createMachine({
  id: 'sidecar',
  initial: 'idle',
  context: {
    lastStatus: null,
  },
  states: {
    idle: {
      on: {
        CHECK: 'checking',
        RESTART: 'restarting',
      },
    },
    checking: {
      on: {
        ONLINE: {
          target: 'online',
          actions: ({ context, event }) => {
            context.lastStatus = event.status
          },
        },
        OFFLINE: {
          target: 'offline',
          actions: ({ context, event }) => {
            context.lastStatus = event.status
          },
        },
      },
    },
    online: {
      on: {
        CHECK: 'checking',
        RESTART: 'restarting',
      },
    },
    offline: {
      on: {
        CHECK: 'checking',
        RESTART: 'restarting',
      },
    },
    restarting: {
      on: {
        ONLINE: 'online',
        OFFLINE: 'offline',
      },
    },
  },
})
