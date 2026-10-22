import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'

export interface PlazaCatalogEntry {
  key: string
  name: string
  platform: string
  offers: Array<{ group: ModelPlazaGroup; model: PlazaModel }>
}

/** Keep vendor identity and each group's pricing intact while merging duplicates. */
export function buildModelCatalog(groups: ModelPlazaGroup[]): PlazaCatalogEntry[] {
  const entries = new Map<string, PlazaCatalogEntry>()
  for (const group of groups) {
    for (const model of group.models) {
      const key = JSON.stringify([model.platform, model.name])
      const entry = entries.get(key) ?? { key, name: model.name, platform: model.platform, offers: [] }
      entry.offers.push({ group, model })
      entries.set(key, entry)
    }
  }
  for (const entry of entries.values()) {
    entry.offers.sort((a, b) =>
      (a.group.user_rate_multiplier ?? a.group.rate_multiplier) -
      (b.group.user_rate_multiplier ?? b.group.rate_multiplier) || a.group.name.localeCompare(b.group.name)
    )
  }
  return [...entries.values()]
}
