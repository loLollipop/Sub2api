import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import { modelProvider } from './modelPlazaProviders'

export interface PlazaCatalogEntry {
  key: string
  name: string
  platform: string
  provider: string
  offers: Array<{ group: ModelPlazaGroup; model: PlazaModel }>
}

/** A catalog entry belongs to one group; never combine pricing across groups. */
export function buildModelCatalog(groups: ModelPlazaGroup[]): PlazaCatalogEntry[] {
  const entries = new Map<string, PlazaCatalogEntry>()
  for (const group of groups) {
    for (const model of group.models) {
      const key = JSON.stringify([group.id, model.platform, model.name])
      if (!entries.has(key)) {
        entries.set(key, { key, name: model.name, platform: model.platform, provider: modelProvider(model), offers: [{ group, model }] })
      }
    }
  }
  return [...entries.values()]
}
