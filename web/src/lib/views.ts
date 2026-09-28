import { useEffect, useState } from 'react'
import { api } from './api'

// Analytics view: a set of dynamic charts + the filter/year context.
export type ViewConfig = {
  kind?: 'brand'
  bars: string[]
  pies: string[]
  tables: string[]
  /** 2-dimension pivot tables, each stored as "rowDim|colDim". */
  crosses?: string[]
  filters: Record<string, string[]>
  /** Values to leave out of every card — the red "Exclude data from:" row. */
  excludes?: Record<string, string[]>
  year: string
  /** Built-in cards (monthly volume, top brands, leaderboard). Absent = on,
   *  so views saved before this flag existed keep showing them. */
  builtins?: boolean
}

/** A report page with no cards at all — the "Empty New Report" starting point. */
export const emptyViewConfig = (): ViewConfig => ({
  kind: 'brand', bars: [], pies: [], tables: [], crosses: [], filters: {}, excludes: {}, year: 'all', builtins: false,
})

/** "New Report", then "New Report 2", … so the sidebar never shows two alike. */
export function nextReportName(views: SavedView[], base = 'New Report'): string {
  const taken = new Set(views.map((v) => v.name))
  if (!taken.has(base)) return base
  for (let i = 2; ; i++) if (!taken.has(`${base} ${i}`)) return `${base} ${i}`
}

// Model Comparison view: the two models + the graph controls.
export type ModelViewConfig = {
  kind: 'model'
  a: number | null
  b: number | null
  year: string
  measure: string
  vals: string[]
  custom?: boolean // true once the user renames it, so we stop auto-naming
}

export type AnyViewConfig = ViewConfig | ModelViewConfig

export type SavedView = {
  id: number
  name: string
  config: any
  created_at: string
  updated_at: string
}

/** Views saved before `kind` existed are Analytics views. */
export const viewKind = (v: SavedView): 'brand' | 'model' =>
  v?.config?.kind === 'model' ? 'model' : 'brand'

// Tiny shared store so the Sidebar and Analytics page see the same list of
// saved views and re-render together on any change. One fetch, many subscribers.
let cache: SavedView[] = []
let loaded = false
const listeners = new Set<() => void>()
const emit = () => listeners.forEach((l) => l())

export async function refreshViews() {
  try { cache = await api.views(); loaded = true; emit() } catch { /* ignore */ }
}

// mutateLocal patches one view in the cache without a refetch — used for
// frequent config auto-saves so the sidebar doesn't flicker.
export function mutateLocal(id: number, fields: Partial<SavedView>) {
  cache = cache.map((v) => (v.id === id ? { ...v, ...fields } : v))
  emit()
}

export function useViews() {
  const [, force] = useState(0)
  useEffect(() => {
    const l = () => force((n) => n + 1)
    listeners.add(l)
    if (!loaded) refreshViews()
    return () => { listeners.delete(l) }
  }, [])

  return {
    views: cache,
    loaded,
    refresh: refreshViews,
    create: async (name: string, config: AnyViewConfig): Promise<SavedView> => {
      const v = await api.createView(name, config)
      await refreshViews()
      return v
    },
    rename: async (id: number, name: string) => {
      const v = await api.updateView(id, { name })
      mutateLocal(id, { name: v.name, updated_at: v.updated_at })
    },
    saveConfig: async (id: number, config: AnyViewConfig, name?: string) => {
      const v = await api.updateView(id, name ? { config, name } : { config })
      mutateLocal(id, { config: v.config, name: v.name, updated_at: v.updated_at })
    },
    remove: async (id: number) => {
      await api.deleteView(id)
      await refreshViews()
    },
  }
}
