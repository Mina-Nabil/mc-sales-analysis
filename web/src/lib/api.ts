// Thin fetch wrapper around the Go JSON API. Same-origin, cookie-authenticated.

export class ApiError extends Error {
  status: number
  data: any
  constructor(message: string, status: number, data: any) {
    super(message)
    this.status = status
    this.data = data
  }
}

// ── In-flight request tracking (drives the global loading bar) ───────────────
// Every API call passes through req(), so a single counter here lets the UI show
// a top progress bar whenever any page or action is waiting on the server.
let inFlight = 0
const loadingListeners = new Set<(n: number) => void>()
function bump(delta: number) {
  inFlight += delta
  loadingListeners.forEach((l) => l(inFlight))
}
export function onLoadingChange(fn: (n: number) => void): () => void {
  loadingListeners.add(fn)
  fn(inFlight)
  return () => { loadingListeners.delete(fn) }
}

async function req(method: string, path: string, body?: any) {
  const init: RequestInit = { method, credentials: 'include', headers: {} }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  bump(1)
  try {
    const res = await fetch('/api/v1' + path, init)
    const text = await res.text()
    const data = text ? JSON.parse(text) : null
    if (!res.ok) throw new ApiError((data && data.error) || res.statusText, res.status, data)
    return data
  } finally {
    bump(-1)
  }
}

export const api = {
  login: (email: string, password: string) => req('POST', '/auth/login', { Email: email, Password: password }),
  logout: () => req('POST', '/auth/logout'),
  me: () => req('GET', '/auth/me'),

  stats: () => req('GET', '/stats'),
  confidence: (year?: number) => req('GET', '/analytics/confidence' + (year ? `?year=${year}` : '')),
  dimensions: () => req('GET', '/analytics/dimensions'),
  matrix: (qs: string) => req('GET', '/analytics/matrix?' + qs),
  exportHref: (qs: string) => '/api/v1/analytics/export.xlsx?' + qs,
  values: (dimension: string, qs = '') => req('GET', `/analytics/values?dimension=${dimension}${qs ? '&' + qs : ''}`),
  agg: (dimension: string, qs = '') => req('GET', `/analytics/agg?dimension=${dimension}${qs ? '&' + qs : ''}`),
  timeseries: (qs = '') => req('GET', '/analytics/timeseries' + (qs ? '?' + qs : '')),

  review: (limit = 100) => req('GET', `/review?limit=${limit}`),
  confirm: (id: number, modelID?: number) => req('POST', `/review/${id}/confirm`, modelID ? { model_id: modelID } : {}),
  reject: (id: number) => req('POST', `/review/${id}/reject`, {}),
  reassign: (id: number, modelID: number) => req('POST', `/review/${id}/reassign`, { model_id: modelID }),
  bulkConfirm: (min: number) => req('POST', '/review/bulk-confirm', { min_confidence: min }),
  resolve: () => req('POST', '/resolve', {}),
  newModelForReview: (aliasId: number, m: any) => req('POST', `/review/${aliasId}/new-model`, m),

  brands: (year?: number | string) => req('GET', '/brands' + (year && year !== 'all' ? `?year=${year}` : '')),
  brandModels: (id: number) => req('GET', `/brands/${id}/models`),
  model: (id: number) => req('GET', `/models/${id}`),
  modelAliases: (id: number) => req('GET', `/models/${id}/aliases`),
  segments: (year?: number | string) => req('GET', '/segments' + (year && year !== 'all' ? `?year=${year}` : '')),
  years: () => req('GET', '/analytics/years'),
  distributors: () => req('GET', '/distributors'),

  createBrand: (b: any) => req('POST', '/brands', b),
  createModel: (m: any) => req('POST', '/models', m),
  editModel: (id: number, fields: any) => req('PATCH', `/models/${id}`, fields),
  mergePreview: (id: number, intoId: number) => req('GET', `/models/${id}/merge-preview?into_id=${intoId}`),
  merge: (id: number, intoId: number) => req('POST', `/models/${id}/merge`, { into_id: intoId }),
  deleteAlias: (id: number) => req('DELETE', `/aliases/${id}`),

  brandQueue: (limit = 100) => req('GET', `/review/brands?limit=${limit}`),
  resolveBrand: (body: any) => req('POST', '/review/brands/resolve', body),

  imports: () => req('GET', '/imports'),
  upload: (file: File, year?: number, month?: number) => {
    const fd = new FormData()
    fd.append('file', file)
    if (year && month) { fd.append('period_year', String(year)); fd.append('period_month', String(month)) }
    return req('POST', '/imports', fd)
  },
  dryRun: (token: string, year?: number, month?: number) =>
    req('GET', `/imports/${token}/dry-run` + (year && month ? `?year=${year}&month=${month}` : '')),
  commit: (token: string, reason: string, year?: number, month?: number) =>
    req('POST', `/imports/${token}/commit`, { reason, period_year: year, period_month: month }),

  views: () => req('GET', '/views'),
  createView: (name: string, config: any) => req('POST', '/views', { name, config }),
  updateView: (id: number, patch: { name?: string; config?: any }) => req('PATCH', `/views/${id}`, patch),
  deleteView: (id: number) => req('DELETE', `/views/${id}`),

  users: () => req('GET', '/users'),
  createUser: (email: string, password: string) => req('POST', '/users', { Email: email, Password: password }),
  updateUser: (id: number, patch: { email?: string; password?: string }) => req('PATCH', `/users/${id}`, patch),
  setUserActive: (id: number, active: boolean) => req('POST', `/users/${id}/active`, { active }),
  deleteUser: (id: number) => req('DELETE', `/users/${id}`),

  settings: () => req('GET', '/settings'),
  saveSettings: (obj: any) => req('PATCH', '/settings', obj),
  changes: () => req('GET', '/changes'),
}

export const fmt = (n: any) => (n == null ? '—' : Number(n).toLocaleString('en-US'))
