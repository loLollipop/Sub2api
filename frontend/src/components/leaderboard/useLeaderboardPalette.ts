import { computed } from 'vue'
import { useChartPalette } from '@/composables/useChartPalette'

export function useLeaderboardPalette() {
  const palette = useChartPalette()
  return computed(() => {
    const colors = palette.value
    // Keep each brand stable across periods and in sync with the share legend.
    const providers: Record<string, string> = {
      openai: colors.accent,
      anthropic: colors.purple,
      gemini: colors.blue,
      grok: colors.series[11],
      deepseek: colors.cyan,
      qwen: colors.series[7],
      kimi: colors.green,
      zhipu: colors.series[5],
      minimax: colors.series[10],
      doubao: colors.amber,
      mistral: colors.series[8],
      meta: colors.series[9],
      hunyuan: colors.isDark ? '#8fc1bb' : '#367a7a',
      baidu: colors.isDark ? '#e899a5' : '#a94354',
      yi: colors.isDark ? '#baa1da' : '#755399',
      spark: colors.isDark ? '#dba0cd' : '#a55190',
      cohere: colors.isDark ? '#a5c18c' : '#587744',
      other: colors.other,
    }
    return { ...colors, providers }
  })
}
