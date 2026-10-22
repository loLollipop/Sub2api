import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { modelProvider, providerLabel } from '../modelPlazaProviders'
import PlazaProviderIcon from '../PlazaProviderIcon.vue'

describe('plaza model brand identity', () => {
  it.each([
    ['deepseek-chat', 'deepseek'], ['DeepSeek-R1-0528', 'deepseek'],
    ['deepseek-ai/DeepSeek-V3.2', 'deepseek'], ['openai/deepseek-ai/DeepSeek-V3', 'deepseek'],
    ['Qwen/Qwen3-235B-A22B-Instruct-2507', 'qwen'], ['qwen3-max', 'qwen'], ['qwq-32b', 'qwen'],
    ['moonshotai/Kimi-K2-Instruct', 'kimi'], ['kimi-k2.5', 'kimi'], ['moonshot-v1-8k', 'kimi'],
    ['z-ai/glm-4.5', 'zhipu'], ['GLM-4.6', 'zhipu'], ['THUDM/GLM-4-32B', 'zhipu'],
    ['MiniMax-M2', 'minimax'], ['minimax/abab6.5-chat', 'minimax'],
    ['doubao-seed-1-6', 'doubao'], ['bytedance/seedream-4.0', 'doubao'],
    ['ernie-4.5-turbo', 'baidu'], ['tencent/Hunyuan-A13B-Instruct', 'hunyuan'],
    ['01-ai/Yi-1.5-34B-Chat', 'yi'], ['spark-4.0-ultra', 'spark'],
    ['gpt-5.4', 'openai'], ['openai/gpt-5', 'openai'], ['o3-mini', 'openai'],
    ['codex-auto-review', 'openai'], ['omni-moderation-latest', 'openai'],
    ['computer-use-preview', 'openai'], ['sora-2-pro', 'openai'],
    ['claude-sonnet-4.5', 'anthropic'], ['gemini-2.5-pro', 'gemini'], ['grok-4', 'grok'],
    ['mistralai/Mistral-Large', 'mistral'], ['meta-llama/Llama-3.3-70B', 'meta'],
  ])('recognizes %s behind an OpenAI-compatible transport as %s', (name, expected) => {
    expect(modelProvider({ name, platform: 'openai' })).toBe(expected)
  })

  it.each(['custom-model', 'test-ada-wrapper', 'myglmproxy', 'gptish-model'])('uses a neutral identity for unknown alias %s', name => {
    expect(modelProvider({ name, platform: 'openai' })).toBe('other')
  })

  it('uses native fallback for aliases without overriding a recognized model', () => {
    expect(modelProvider({ name: 'my-alias', platform: 'deepseek' })).toBe('deepseek')
    expect(modelProvider({ name: 'qwen3-max', platform: 'deepseek' })).toBe('qwen')
    expect(modelProvider({ name: 'custom-model', platform: 'composite' })).toBe('other')
    expect(providerLabel('qwen')).toBe('Qwen')
  })

  it.each(['deepseek-chat', 'qwen3-max', 'kimi-k2', 'glm-4.5', 'minimax-m2', 'doubao-seed-1-6', 'ernie-4.5', 'hunyuan-a13b'])('renders a real brand logo for %s', name => {
    const icon = mount(PlazaProviderIcon, { props: { platform: 'openai', model: name, size: 'lg' } })
    expect(icon.attributes('data-provider')).toBe(modelProvider({ name, platform: 'openai' }))
    expect(icon.find('svg path').exists()).toBe(true)
    expect(icon.find('.model-icon-fallback').exists()).toBe(false)
    icon.unmount()
  })

  it('reacts to model changes without retaining the transport logo', async () => {
    const icon = mount(PlazaProviderIcon, { props: { platform: 'openai', model: 'gpt-5.4' } })
    const gptPath = icon.get('path').attributes('d')
    await icon.setProps({ model: 'deepseek-chat' })
    expect(icon.attributes('data-provider')).toBe('deepseek')
    expect(icon.get('path').attributes('d')).not.toBe(gptPath)
    await icon.setProps({ model: 'custom-alias' })
    expect(icon.attributes('data-provider')).toBe('other')
    expect(icon.find('svg rect').exists()).toBe(true)
  })
})
