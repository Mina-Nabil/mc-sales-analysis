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

async function req(method: string, path: string, body?: any) {
  const init: RequestInit = { method, credentials: 'include', headers: {} }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  const res = await fetch('/api/v1' + path, init)
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) throw new ApiError((data && data.error) || res.statusText, res.status, data)
  return data
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
  upload: (file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    return req('POST', '/imports', fd)
  },
  dryRun: (token: string) => req('GET', `/imports/${token}/dry-run`),
  commit: (token: string, reason: string) => req('POST', `/imports/${token}/commit`, { reason }),

  settings: () => req('GET', '/settings'),
  saveSettings: (obj: any) => req('PATCH', '/settings', obj),
  changes: () => req('GET', '/changes'),
}

export const fmt = (n: any) => (n == null ? '—' : Number(n).toLocaleString('en-US'))
