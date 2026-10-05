<template>
  <section class="api-preview" :aria-label="t('home.landing.previewTitle')" data-testid="home-api-preview">
    <div class="api-preview__header">
      <div class="api-preview__window-dots" aria-hidden="true">
        <span></span><span></span><span></span>
      </div>
      <span class="api-preview__window-title">
        OpenAI <span aria-hidden="true">·</span>
        {{ activeProtocol === 'responses' ? 'Responses API' : 'Chat Completions' }}
      </span>
      <span class="api-preview__language">cURL</span>
    </div>

    <div class="api-preview__editor">
      <div class="api-preview__toolbar">
        <div class="api-preview__protocols" role="group" :aria-label="t('home.landing.protocolLabel')">
          <button
            v-for="protocol in protocols"
            :key="protocol.id"
            type="button"
            :aria-pressed="activeProtocol === protocol.id"
            :class="{ 'is-active': activeProtocol === protocol.id }"
            :data-testid="`home-protocol-${protocol.id}`"
            @click="activeProtocol = protocol.id"
          >
            {{ protocol.label }}
          </button>
        </div>
        <button
          type="button"
          class="api-preview__copy"
          data-testid="home-copy-example"
          :aria-label="t('home.landing.copyExample')"
          @click="copyToClipboard(example)"
        >
          <Icon :name="copied ? 'check' : 'copy'" size="sm" aria-hidden="true" />
          <span aria-live="polite">{{ copied ? t('home.landing.copied') : t('home.landing.copy') }}</span>
        </button>
      </div>
      <pre class="api-preview__code" tabindex="0" :aria-label="t('home.landing.copyExample')"><code><span v-for="(token, index) in codeTokens" :key="index" :class="token.kind && `syntax-${token.kind}`">{{ token.text }}</span></code><span class="api-preview__cursor" aria-hidden="true" data-testid="home-code-cursor"></span></pre>
      <div class="api-preview__editor-footer">
        <span><Icon name="key" size="sm" aria-hidden="true" /> {{ t('home.landing.replaceKey') }}</span>
        <span class="api-preview__method">POST</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{ apiBaseUrl: string }>()
const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()
const protocols = [
  { id: 'responses', label: 'Responses' },
  { id: 'chat', label: 'Chat Completions' },
] as const
const activeProtocol = ref<(typeof protocols)[number]['id']>('responses')
const endpoint = computed(() => activeProtocol.value === 'responses' ? '/v1/responses' : '/v1/chat/completions')
const example = computed(() => {
  const url = new URL(props.apiBaseUrl || window.location.origin)
  const basePath = url.pathname.replace(/\/+$/, '').replace(/\/v1$/, '')
  url.pathname = `${basePath}${endpoint.value}`
  url.hash = ''
  // Keep the copied shell literal safe if a configured URL contains a quote.
  const requestUrl = url.toString().replace(/'/g, '%27')
  const body = activeProtocol.value === 'responses'
    ? { model: 'YOUR_MODEL', input: 'Hello!' }
    : { model: 'YOUR_MODEL', messages: [{ role: 'user', content: 'Hello!' }] }
  return [
    `curl '${requestUrl}' \\`,
    '  -H "Authorization: Bearer YOUR_API_KEY" \\',
    '  -H "Content-Type: application/json" \\',
    `  -d '${JSON.stringify(body, null, 2)}'`,
  ].join('\n')
})

// Small cURL/JSON lexer: Vue escapes every token, including configured URLs.
// No HTML injection or syntax-highlighting dependency is needed for this card.
const codeTokens = computed(() => {
  const source = example.value
  const pattern = /"(?:\\.|[^"\\])*"|'https?:\/\/[^'\n]*'|https?:\/\/[^\s'"]+|\bcurl\b|\b(?:true|false|null)\b|\bYOUR_[A-Z_]+\b|-[Hd]\b/g
  const tokens: { text: string; kind: string }[] = []
  let cursor = 0
  for (const match of source.matchAll(pattern)) {
    const index = match.index
    if (index > cursor) tokens.push({ text: source.slice(cursor, index), kind: '' })
    const text = match[0]
    const kind = text === 'curl' ? 'command'
      : text.startsWith('-') ? 'option'
      : /^'?https?:/.test(text) ? 'url'
      : text.startsWith('YOUR_') || text.includes('"YOUR_') ? 'placeholder'
      : text.startsWith('"') && /^\s*:/.test(source.slice(index + text.length)) ? 'key'
      : text.startsWith('"') ? 'string' : 'literal'
    tokens.push({ text, kind })
    cursor = index + text.length
  }
  if (cursor < source.length) tokens.push({ text: source.slice(cursor), kind: '' })
  return tokens
})
</script>

<style scoped>
.api-preview {
  overflow: hidden;
  border: 1px solid var(--home-line);
  border-radius: 20px;
  background: var(--home-panel);
  box-shadow: 0 2px 4px #1c1c1e03, 0 24px 64px -28px var(--home-shadow);
}
.api-preview__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 57px;
  padding: 12px 23px;
  border-bottom: 1px solid var(--home-line);
  background: var(--home-code-bg);
}
.api-preview__window-dots { display: flex; gap: 7px; flex-shrink: 0; }
.api-preview__window-dots span { width: 9px; height: 9px; border-radius: 50%; background: #fa6c65; }
.api-preview__window-dots span:nth-child(2) { background: #edbc45; }
.api-preview__window-dots span:nth-child(3) { background: #46b983; }
.api-preview__window-title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--home-muted); font-size: 12px; }
.api-preview__window-title > span { padding-inline: 3px; }
.api-preview__language { flex-shrink: 0; border: 1px solid var(--home-line); border-radius: 6px; padding: 4px 8px; color: var(--home-muted); background: var(--home-panel); font: 600 10px/1.3 ui-monospace, monospace; }
.api-preview__editor { min-width: 0; }
.api-preview__toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; padding: 18px 22px 0; }
.api-preview__protocols { display: flex; flex-wrap: wrap; gap: 4px; }
.api-preview__protocols button { min-height: 36px; padding: 7px 12px; border: 1px solid transparent; border-radius: 7px; color: var(--home-muted); font-size: 11px; font-weight: 550; transition: color 150ms, background 150ms; }
.api-preview__protocols button.is-active { background: var(--home-accent-soft); color: var(--home-accent); }
.api-preview__protocols button:hover { color: var(--home-accent); }
.api-preview__copy { display: flex; align-items: center; gap: 7px; min-height: 36px; padding: 7px; border-radius: 6px; color: var(--home-muted); font-size: 11px; }
.api-preview__copy:hover { background: var(--home-code-bg); color: var(--home-ink); }
.api-preview__code { height: 290px; max-width: 100%; overflow: auto; margin: 0; padding: 24px 27px 28px; color: var(--home-ink); font: 13px/1.85 'Cascadia Code', 'SFMono-Regular', Consolas, monospace; tab-size: 2; }
.syntax-command, .syntax-literal { color: var(--syntax-command); font-weight: 600; }
.syntax-url { color: var(--syntax-url); }
.syntax-key { color: var(--syntax-key); }
.syntax-string { color: var(--syntax-string); }
.syntax-placeholder { color: var(--syntax-placeholder); }
.syntax-option { color: var(--home-muted); }
.api-preview__cursor { display: inline-block; width: 7px; height: 1.15em; margin-left: 6px; vertical-align: -0.15em; background: var(--home-accent); animation: home-cursor-blink 1.2s steps(1, end) infinite; }
@keyframes home-cursor-blink { 0%, 55%, 100% { opacity: 1; } 56%, 99% { opacity: 0; } }
.api-preview__editor-footer { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 8px; padding: 13px 24px; border-top: 1px solid var(--home-line); color: var(--home-muted); font-size: 10px; }
.api-preview__editor-footer > span:first-child { display: flex; align-items: center; gap: 7px; }
.api-preview__method { font: 600 10px/1.7 ui-monospace, monospace; letter-spacing: 0.06em; }
.api-preview :is(button, pre):focus-visible { outline: 2px solid var(--home-accent); outline-offset: 3px; }
@media (max-width: 720px) {
  .api-preview__header { gap: 9px; padding-inline: 17px; }
  .api-preview__window-dots { gap: 5px; }
  .api-preview__window-dots span { width: 7px; height: 7px; }
  .api-preview__window-title { font-size: 11px; }
  .api-preview__code { padding-inline: 20px; font-size: 11px; }
  .api-preview__toolbar { padding-inline: 14px; }
  .api-preview__editor-footer { padding-inline: 20px; }
}
@media (prefers-reduced-motion: reduce) { .api-preview * { transition: none !important; } .api-preview__cursor { animation: none; opacity: 1; } }
</style>
