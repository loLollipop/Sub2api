import type { PlazaModel } from '@/api/modelPlaza'

const providers: Record<string, string> = {
  openai: 'OpenAI', anthropic: 'Anthropic', gemini: 'Google', grok: 'Grok',
  deepseek: 'DeepSeek', qwen: 'Qwen', kimi: 'Kimi', zhipu: 'GLM', minimax: 'MiniMax',
  doubao: 'Doubao', baidu: 'ERNIE', hunyuan: 'Hunyuan', yi: 'Yi', spark: 'Spark',
  mistral: 'Mistral', meta: 'Meta', cohere: 'Cohere', other: 'Other',
}

export function providerLabel(provider: string): string {
  return Object.prototype.hasOwnProperty.call(providers, provider) ? providers[provider] : provider
}

// Brand is presentation metadata. Never use it in pricing or request routing:
// an OpenAI-compatible channel can serve any of these model families.
const families: Array<[string, RegExp]> = [
  ['deepseek', /^deepseek(?:[-_.\s\d]|$)/],
  ['qwen', /^(?:qwen|qwq|qvq)(?:[-_.\s\d]|$)/],
  ['kimi', /^(?:kimi|moonshot)(?:[-_.\s\d]|$)/],
  ['zhipu', /^(?:(?:chat)?glm|cogview|cogvideo)(?:[-_.\s\d]|$)/],
  ['minimax', /^(?:minimax(?:[-_.\s\d]|$)|abab\d)/],
  ['doubao', /^(?:doubao|seed|seedream|seedance)(?:[-_.\s\d]|$)/],
  ['baidu', /^(?:ernie|wenxin)(?:[-_.\s\d]|$)/],
  ['hunyuan', /^hunyuan(?:[-_.\s\d]|$)/],
  ['yi', /^yi(?:[-_.\s\d]|$)/],
  ['spark', /^spark(?:[-_.\s\d]|$)/],
  ['anthropic', /^claude(?:[-_.\s\d]|$)/],
  ['gemini', /^(?:gemini|gemma|learnlm|imagen|veo)(?:[-_.\s\d]|$)/],
  ['grok', /^grok(?:[-_.\s\d]|$)/],
  ['mistral', /^(?:mistral|mixtral|codestral|pixtral|voxtral|magistral|ministral)(?:[-_.\s\d]|$)/],
  ['meta', /^llama(?:[-_.\s\d]|$)/],
  ['cohere', /^(?:command(?:[-_.\s]|$)|c4ai[-_]|embed[-_](?:english|multilingual))/],
  ['openai', /^(?:codex|omni[-_]moderation|computer[-_]use|sora)(?:[-_.\s\d]|$)/],
  ['openai', /^(?:gpt(?:[-_.\s\d]|$)|chatgpt(?:[-_.\s\d]|$)|o[134](?:[-_.\s]|$)|dall[-_]?e(?:[-_.\s\d]|$)|whisper(?:[-_.\s\d]|$)|tts[-_]1(?:[-_.\s]|$)|text[-_](?:embedding|moderation)(?:[-_.\s\d]|$))/],
]

export function modelProvider(model: Pick<PlazaModel, 'name' | 'platform'>): string {
  // Inspect the actual model segment first, then explicit provider namespaces.
  const parts = model.name.trim().toLowerCase().split(/[/:\\]+/).filter(Boolean)
  for (const part of [...parts].reverse()) {
    for (const [provider, pattern] of families) {
      if (pattern.test(part)) return provider
    }
  }
  for (const part of parts) {
    const namespace = part.replace(/[-_]ai$/, '')
    if (Object.prototype.hasOwnProperty.call(providers, namespace) && namespace !== 'other' && namespace !== 'openai') return namespace
    if (namespace === 'moonshot' || namespace === 'moonshotai') return 'kimi'
    if (namespace === 'z-ai' || namespace === 'zhipuai') return 'zhipu'
  }
  // A native provider can identify its custom aliases. A generic protocol
  // platform cannot identify an unknown model's brand.
  const native = model.platform.toLowerCase()
  return Object.prototype.hasOwnProperty.call(providers, native) && native !== 'openai' && native !== 'other' ? native : 'other'
}
