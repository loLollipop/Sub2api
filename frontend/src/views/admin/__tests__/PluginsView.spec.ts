import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PluginsView from '../PluginsView.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { PluginUISession } from '@/api/admin'

enableAutoUnmount(afterEach)

const {
  listPlugins,
  uploadPlugin,
  enablePlugin,
  savePluginConfig,
  getPluginConfig,
  createUISession,
  stepUpRun,
} = vi.hoisted(() => ({
  listPlugins: vi.fn(),
  uploadPlugin: vi.fn(),
  enablePlugin: vi.fn(),
  savePluginConfig: vi.fn(),
  getPluginConfig: vi.fn(),
  createUISession: vi.fn(),
  stepUpRun: vi.fn((action: () => Promise<unknown>) => action()),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    plugins: {
      list: listPlugins,
      upload: uploadPlugin,
      enable: enablePlugin,
      disable: vi.fn(),
      remove: vi.fn(),
      getConfig: getPluginConfig,
      saveConfig: savePluginConfig,
      test: vi.fn().mockResolvedValue({ success: true, message: 'ok', latency_ms: 1 }),
      createUISession,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: stepUpRun }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const plugin = {
  id: 7,
  plugin_key: 'local.test.transport',
  name: 'Test Transport',
  version: '1.0.0',
  description: '',
  author: 'test',
  manifest: {
    schema_version: 1,
    id: 'local.test.transport',
    name: 'Test Transport',
    version: '1.0.0',
    requires: {
      sub2api: '>=0.1.0',
      plugin_protocol: 1,
      transport_api: 1,
      ui_bridge: 1,
    },
    capabilities: [],
    ui: { entrypoint: 'ui/index.html' },
  },
  binary_sha256: 'a'.repeat(64),
  signature_status: 'trusted' as const,
  state: 'disabled' as const,
  last_error: '',
  installed_at: '2026-08-22T00:00:00Z',
  updated_at: '2026-08-22T00:00:00Z',
  bindings: [
    {
      id: 1,
      plugin_id: 7,
      capability: 'openai.oauth.outbound_transport.v1',
      platform: 'openai',
      account_type: 'oauth',
      enabled: false,
      rollout_percent: 100,
    },
  ],
  compatibility: {
    compatible: true,
    tested: true,
    status: 'compatible' as const,
    message: '',
    current_sub2api_version: '0.1.0',
    required_sub2api_version: '>=0.1.0',
    recommended_sub2api_version: '0.1.0',
    plugin_protocol: 1,
    transport_api: 1,
    ui_bridge: 1,
  },
  runtime_healthy: false,
  runtime_message: '',
}

function mountView() {
  return mount(PluginsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { template: '<div><slot /></div>' },
        Icon: true,
        TotpStepUpDialog: true,
      },
    },
  })
}

function documentBridge(frame: HTMLIFrameElement, source = { postMessage: vi.fn() }) {
  Object.defineProperty(frame, 'contentWindow', { value: source, configurable: true })
  const port = {
    onmessage: null as ((event: MessageEvent) => void) | null,
    postMessage: vi.fn(), start: vi.fn(), close: vi.fn(),
  }
  function send(data: Record<string, unknown>, echo = true) {
    const message = { source: 'sub2api-plugin-ui', bridge_token: 'bridge', ...data }
    window.dispatchEvent(new MessageEvent('message', {
      source: source as unknown as Window, origin: 'null', data: message,
    }))
    const relay = port.postMessage.mock.calls.at(-1)?.[0]
    if (echo && relay?.type === 'sub2api.plugin.relay') {
      port.onmessage?.(new MessageEvent('message', { data: { type: 'sub2api.plugin.request', message: relay.message } }))
    }
  }
  window.dispatchEvent(new MessageEvent('message', {
    source: source as unknown as Window, origin: 'null',
    data: { source: 'sub2api-plugin-ui', bridge_token: 'bridge', type: 'sub2api.plugin.connect' },
    ports: [port as unknown as MessagePort],
  }))
  port.postMessage.mockClear()
  return { source, port, send }
}

describe('管理员插件页二次验证', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => action())
    listPlugins.mockResolvedValue([plugin])
    uploadPlugin.mockResolvedValue(plugin)
    enablePlugin.mockResolvedValue(plugin)
    savePluginConfig.mockResolvedValue({ enabled: true })
    getPluginConfig.mockResolvedValue({})
    createUISession.mockResolvedValue({
      url: '/api/v1/plugin-ui/token/index.html#bridge_token=bridge',
      bridge_token: 'bridge',
      ui_bridge_version: 1,
      expires_at: '2026-08-22T01:00:00Z',
    })
  })

  it('启用插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.plugins.enable'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(enablePlugin).toHaveBeenCalledWith(7, 100, false)
  })

  it.each([7, 8])('keeps the latest session when reopening configuration for plugin %s', async (id) => {
    let resolveOld!: (session: PluginUISession) => void
    createUISession.mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveOld = resolve }))
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8, name: 'Other Transport' }])
    const wrapper = mountView()
    await flushPromises()
    const buttons = wrapper.findAll('button').filter(item => item.text() === 'admin.plugins.configure')
    await buttons[0].trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await buttons[id === 7 ? 0 : 1].trigger('click')
    await flushPromises()
    const currentURL = wrapper.get('iframe').attributes('src')

    resolveOld({ url: '/obsolete-session', bridge_token: 'old', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    expect(wrapper.get('iframe').attributes('src')).toBe(currentURL)
  })

  it('ignores an old session error while the reopened configuration is loading', async () => {
    let rejectOld!: (error: Error) => void
    let resolveCurrent!: (session: PluginUISession) => void
    createUISession
      .mockReturnValueOnce(new Promise((_, reject) => { rejectOld = reject }))
      .mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveCurrent = resolve }))
    const wrapper = mountView()
    await flushPromises()
    const button = wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!
    await button.trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await button.trigger('click')
    rejectOld(new Error('obsolete failure'))
    await flushPromises()

    expect(wrapper.text()).not.toContain('obsolete failure')
    expect(wrapper.text()).toContain('admin.plugins.loadingUI')
    resolveCurrent({ url: '/current-session', bridge_token: 'current', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    await wrapper.get('iframe').trigger('load')
    expect(wrapper.get('iframe').attributes('src')).toBe('/current-session')
    expect(wrapper.text()).not.toContain('admin.plugins.loadingUI')
  })

  it('does not restore a session after the configuration dialog closes', async () => {
    let resolveSession!: (session: PluginUISession) => void
    createUISession.mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveSession = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    resolveSession({ url: '/closed-session', bridge_token: 'old', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    expect(wrapper.find('iframe').exists()).toBe(false)
  })

  it('上传插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['plugin'], 'transport.s2plugin', { type: 'application/zip' })],
    })

    await input.trigger('change')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(uploadPlugin).toHaveBeenCalledTimes(1)
  })

  it.each(['switch', 'reload'])('never delivers an old config response after iframe %s with a reused request id', async (change) => {
    let resolveOld!: (config: Record<string, unknown>) => void
    let resolveNew!: (config: Record<string, unknown>) => void
    savePluginConfig
      .mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
      .mockReturnValueOnce(new Promise(resolve => { resolveNew = resolve }))
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8, name: 'Other Transport' }])
    const wrapper = mountView()
    await flushPromises()
    const buttons = wrapper.findAll('button').filter(item => item.text() === 'admin.plugins.configure')
    await buttons[0].trigger('click')
    await flushPromises()
    const old = documentBridge(wrapper.get('iframe').element)
    await wrapper.get('iframe').trigger('load')
    const request = { type: 'config.save', request_id: 'same-id', config: { enabled: true } }
    old.send(request)
    await flushPromises()
    if (change === 'switch') {
      wrapper.getComponent(BaseDialog).vm.$emit('close')
      await buttons[1].trigger('click')
      await flushPromises()
    } else {
      await wrapper.get('iframe').trigger('load')
      await flushPromises()
    }
    const current = documentBridge(wrapper.get('iframe').element, change === 'reload' ? old.source : undefined)
    await wrapper.get('iframe').trigger('load')
    current.send(request)
    await flushPromises()
    current.port.postMessage.mockClear()
    resolveOld({ oldSecret: 'must-not-cross' })
    await flushPromises()
    expect(current.port.postMessage).not.toHaveBeenCalled()
    resolveNew({ current: true })
    await flushPromises()
    expect(current.port.postMessage).toHaveBeenCalledTimes(1)
    expect(current.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ config: { current: true } }))
    expect(old.port.close).toHaveBeenCalled()
    expect(current.source.postMessage).not.toHaveBeenCalled()
    expect(savePluginConfig.mock.calls.map(call => call[0])).toEqual([7, change === 'switch' ? 8 : 7])
  })

  it('does not run a deferred step-up write after its configuration document is closed', async () => {
    let deferredAction!: () => Promise<unknown>
    stepUpRun.mockImplementationOnce((action: () => Promise<unknown>) => {
      deferredAction = action
      return new Promise(() => {})
    })
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8, name: 'Other Transport' }])
    const wrapper = mountView()
    await flushPromises()
    const buttons = wrapper.findAll('button').filter(item => item.text() === 'admin.plugins.configure')
    await buttons[0].trigger('click')
    await flushPromises()
    const bridge = documentBridge(wrapper.get('iframe').element)
    bridge.send({ type: 'config.save', request_id: 'deferred', config: { enabled: true } })
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await buttons[1].trigger('click')
    await flushPromises()
    await deferredAction()
    expect(savePluginConfig).not.toHaveBeenCalled()
  })

  it('requires an echo from the owning document before executing a legacy request', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises()
    const bridge = documentBridge(wrapper.get('iframe').element)
    bridge.send({ type: 'config.save', request_id: 'verify-document', config: { enabled: true } }, false)
    await flushPromises()
    expect(savePluginConfig).not.toHaveBeenCalled()
    const relay = bridge.port.postMessage.mock.calls.at(-1)![0]
    bridge.port.onmessage!(new MessageEvent('message', { data: { type: 'sub2api.plugin.request', message: relay.message } }))
    await flushPromises()
    expect(savePluginConfig).toHaveBeenCalledWith(7, { enabled: true })
  })

  it('keeps an in-flight config reply on the original document port before navigation load', async () => {
    let resolveConfig!: (config: Record<string, unknown>) => void
    getPluginConfig.mockReturnValueOnce(new Promise(resolve => { resolveConfig = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises()
    const original = documentBridge(wrapper.get('iframe').element)
    await wrapper.get('iframe').trigger('load')
    original.send({ type: 'config.load', request_id: 'old-document' })
    await flushPromises()
    original.port.postMessage.mockClear()
    // WindowProxy identity survives navigation. A replacement document cannot
    // take over the private port even before its load event has fired.
    const replacement = documentBridge(wrapper.get('iframe').element, original.source)
    expect(replacement.port.close).toHaveBeenCalledOnce()
    resolveConfig({ secret: 'original-document-only' })
    await flushPromises()
    expect(original.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ config: { secret: 'original-document-only' } }))
    expect(replacement.port.postMessage).not.toHaveBeenCalled()
    expect(original.source.postMessage).not.toHaveBeenCalled()
  })

  it('ignores configuration requests with no document channel', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises()
    const source = { postMessage: vi.fn() }
    Object.defineProperty(wrapper.get('iframe').element, 'contentWindow', { value: source })
    for (const type of ['config.load', 'config.save', 'config.test']) {
      window.dispatchEvent(new MessageEvent('message', {
        source: source as unknown as Window, origin: 'null',
        data: { source: 'sub2api-plugin-ui', bridge_token: 'bridge', type, request_id: type, config: {} },
      }))
    }
    await flushPromises()
    expect(getPluginConfig).not.toHaveBeenCalled()
    expect(savePluginConfig).not.toHaveBeenCalled()
    expect(source.postMessage).not.toHaveBeenCalled()
  })

  it('invalidates a pending step-up on document pagehide before load and refuses a replacement channel', async () => {
    let deferredAction!: () => Promise<unknown>
    stepUpRun.mockImplementationOnce((action: () => Promise<unknown>) => {
      deferredAction = action
      return new Promise(() => {})
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises()
    const original = documentBridge(wrapper.get('iframe').element)
    await wrapper.get('iframe').trigger('load')
    original.send({ type: 'config.save', request_id: 'awaiting-step-up', config: { enabled: true } })
    original.port.onmessage!(new MessageEvent('message', { data: { type: 'sub2api.plugin.disconnect' } }))
    const replacement = documentBridge(wrapper.get('iframe').element, original.source)
    expect(original.port.close).toHaveBeenCalledOnce()
    expect(replacement.port.close).toHaveBeenCalledOnce()
    await deferredAction()
    expect(savePluginConfig).not.toHaveBeenCalled()
  })

  it('recreates a session when its first document disconnects before the initial load', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises()
    const oldFrame = wrapper.get('iframe')
    const original = documentBridge(oldFrame.element)
    let resolveSession!: (session: PluginUISession) => void
    createUISession.mockReturnValueOnce(new Promise<PluginUISession>(resolve => { resolveSession = resolve }))
    original.port.onmessage!(new MessageEvent('message', { data: { type: 'sub2api.plugin.disconnect' } }))
    // A queued load from the retired frame must not mark the replacement loaded.
    await oldFrame.trigger('load')
    resolveSession({ url: '/fresh-session', bridge_token: 'bridge', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    const current = documentBridge(wrapper.get('iframe').element)
    await wrapper.get('iframe').trigger('load')
    current.send({ type: 'config.load', request_id: 'restored' })
    await flushPromises()
    expect(createUISession).toHaveBeenCalledTimes(2)
    expect(current.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ request_id: 'restored', ok: true }))
  })
})
