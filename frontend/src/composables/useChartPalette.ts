import { computed, ref } from 'vue'
import { useMutationObserver } from '@vueuse/core'

// Keep the same color order for chart segments and their legends in both themes.
const lightSeries = [
  '#397fde', '#e56c45', '#28a074', '#a16bd0',
  '#dfaa41', '#d87099', '#329fac', '#7375d7',
  '#b68a54', '#6d9d45', '#c36563', '#697f9f'
]
const darkSeries = [
  '#8ab5ff', '#f2a181', '#70c9a1', '#bd99ea',
  '#e6be71', '#e9a2bd', '#77c3ce', '#a4a5f3',
  '#d1ae84', '#adcb80', '#e69c97', '#a2b3cc'
]

export function useChartPalette() {
  const dark = ref(document.documentElement.classList.contains('dark'))
  useMutationObserver(document.documentElement, () => {
    dark.value = document.documentElement.classList.contains('dark')
  }, { attributes: true, attributeFilter: ['class'] })

  return computed(() => {
    const series = dark.value ? darkSeries : lightSeries
    return {
      isDark: dark.value,
      series,
      accent: series[0],
      blue: series[0],
      orange: series[1],
      green: series[2],
      purple: series[3],
      amber: series[4],
      cyan: series[6],
      other: dark.value ? '#a4abb8' : '#808898',
      text: dark.value ? '#b4bac6' : '#646c7a',
      grid: dark.value ? '#313640' : '#e2e5eb',
      tooltip: {
        backgroundColor: dark.value ? '#1d2027' : '#ffffff',
        titleColor: dark.value ? '#eceef2' : '#171a21',
        bodyColor: dark.value ? '#a4abb8' : '#5d6573',
        borderColor: dark.value ? '#313640' : '#e2e5eb',
        borderWidth: 1
      }
    }
  })
}
