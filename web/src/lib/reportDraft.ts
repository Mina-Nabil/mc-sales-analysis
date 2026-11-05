import { emptyViewConfig, type CrossSpec, type ViewConfig } from './views'

export const WIZARD_STEPS = ['name', 'source', 'cards', 'exclude'] as const
export type WizardStep = typeof WIZARD_STEPS[number]

export const STEP_META: { id: WizardStep; label: string; title: string; subtitle: string }[] = [
  { id: 'name', label: 'Name', title: 'Name the report', subtitle: 'This is how it will appear in the sidebar.' },
  { id: 'source', label: 'Data', title: 'Choose the data', subtitle: 'These filters decide which registrations are in the report. Leave empty to include everything.' },
  { id: 'cards', label: 'Cards', title: 'Show the data', subtitle: 'Add tables and charts. Each one breaks the same data down a different way.' },
  { id: 'exclude', label: 'Exclude', title: 'Exclude values', subtitle: 'Drop values you do not want in the totals (for example Unknown, or a brand that is out of scope). Period totals still reconcile.' },
]

export type ReportDraft = {
  name: string
  year: string
  filters: Record<string, string[]>
  excludes: Record<string, string[]>
  bars: string[]
  pies: string[]
  tables: string[]
  crosses: CrossSpec[]
  builtins: boolean
}

export type DraftPatch = Partial<ReportDraft> | ((prev: ReportDraft) => Partial<ReportDraft>)

const DEFAULT_CROSS: CrossSpec = { dim1: 'model', dim2: 'model_year' }

function asCross(c: any): CrossSpec {
  if (typeof c === 'string') {
    const [a, b] = c.split('|')
    return { dim1: a || DEFAULT_CROSS.dim1, dim2: b || DEFAULT_CROSS.dim2 }
  }
  return {
    dim1: c?.dim1 || DEFAULT_CROSS.dim1,
    dim2: c?.dim2 || DEFAULT_CROSS.dim2,
    values2: Array.isArray(c?.values2) ? c.values2 : undefined,
  }
}

export function sanitizeFilters(f: Record<string, string[]> | undefined): Record<string, string[]> {
  if (!f || typeof f !== 'object') return {}
  return Object.fromEntries(Object.entries(f).filter(([, v]) => Array.isArray(v) && v.length))
}

export function emptyDraft(): ReportDraft {
  const c = emptyViewConfig()
  return {
    name: '',
    year: c.year,
    filters: {},
    excludes: {},
    bars: [],
    pies: [],
    tables: [],
    crosses: [],
    builtins: false,
  }
}

export function draftFromView(name: string, cfg: ViewConfig): ReportDraft {
  return {
    name,
    year: typeof cfg.year === 'string' ? cfg.year : 'all',
    filters: sanitizeFilters(cfg.filters),
    excludes: sanitizeFilters(cfg.excludes),
    bars: Array.isArray(cfg.bars) ? cfg.bars.filter((x) => typeof x === 'string') : [],
    pies: Array.isArray(cfg.pies) ? cfg.pies.filter((x) => typeof x === 'string') : [],
    tables: Array.isArray(cfg.tables) ? cfg.tables.filter((x) => typeof x === 'string') : [],
    crosses: Array.isArray(cfg.crosses) ? cfg.crosses.map(asCross) : [],
    builtins: cfg.builtins !== false,
  }
}

export function configFromDraft(d: ReportDraft): ViewConfig {
  return {
    kind: 'brand',
    bars: d.bars,
    pies: d.pies,
    tables: d.tables,
    crosses: d.crosses,
    filters: sanitizeFilters(d.filters),
    excludes: sanitizeFilters(d.excludes),
    year: d.year || 'all',
    builtins: d.builtins,
  }
}

function storageKey(viewId: number | null) {
  return viewId == null ? 'mc.report-draft.new' : `mc.report-draft.${viewId}`
}

export function loadReportDraft(viewId: number | null): ReportDraft | null {
  try {
    const raw = sessionStorage.getItem(storageKey(viewId))
    if (!raw) return null
    const p = JSON.parse(raw)
    if (!p || typeof p !== 'object') return null
    return {
      name: typeof p.name === 'string' ? p.name : '',
      year: typeof p.year === 'string' ? p.year : 'all',
      filters: sanitizeFilters(p.filters),
      excludes: sanitizeFilters(p.excludes),
      bars: Array.isArray(p.bars) ? p.bars.filter((x: any) => typeof x === 'string') : [],
      pies: Array.isArray(p.pies) ? p.pies.filter((x: any) => typeof x === 'string') : [],
      tables: Array.isArray(p.tables) ? p.tables.filter((x: any) => typeof x === 'string') : [],
      crosses: Array.isArray(p.crosses) ? p.crosses.map(asCross) : [],
      builtins: !!p.builtins,
    }
  } catch {
    return null
  }
}

export function saveReportDraft(viewId: number | null, d: ReportDraft) {
  try { sessionStorage.setItem(storageKey(viewId), JSON.stringify(d)) } catch { /* quota / private mode */ }
}

export function clearReportDraft(viewId: number | null) {
  try { sessionStorage.removeItem(storageKey(viewId)) } catch { /* ignore */ }
}

export function wizardPath(viewId: number | null, step: WizardStep): string {
  const base = viewId == null ? '/analytics/new' : `/analytics/view/${viewId}/edit`
  return step === 'name' ? base : `${base}/${step}`
}

export function parseWizardStep(raw?: string): WizardStep | null {
  if (!raw) return 'name'
  if ((WIZARD_STEPS as readonly string[]).includes(raw)) return raw as WizardStep
  return null
}

export function nextStep(s: WizardStep): WizardStep | null {
  const i = WIZARD_STEPS.indexOf(s)
  return i < 0 || i === WIZARD_STEPS.length - 1 ? null : WIZARD_STEPS[i + 1]
}

export function prevStep(s: WizardStep): WizardStep | null {
  const i = WIZARD_STEPS.indexOf(s)
  return i <= 0 ? null : WIZARD_STEPS[i - 1]
}

export function hasCards(d: ReportDraft): boolean {
  return d.builtins || d.bars.length > 0 || d.pies.length > 0 || d.tables.length > 0 || d.crosses.length > 0
}

export function setFilterMap(map: Record<string, string[]>, dim: string, values: string[]): Record<string, string[]> {
  const next = { ...map }
  if (values.length) next[dim] = values
  else delete next[dim]
  return next
}
