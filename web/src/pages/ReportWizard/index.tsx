import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate, useParams } from 'react-router-dom'
import { Button, Card } from '@/components/ui'
import { useViews, viewKind, type ViewConfig } from '@/lib/views'
import {
  STEP_META,
  clearReportDraft,
  configFromDraft,
  draftFromView,
  emptyDraft,
  hasCards,
  loadReportDraft,
  nextStep,
  parseWizardStep,
  prevStep,
  saveReportDraft,
  wizardPath,
  type DraftPatch,
  type ReportDraft,
  type WizardStep,
} from '@/lib/reportDraft'
import { cn } from '@/lib/cn'
import { StepName } from './StepName'
import { StepSource } from './StepSource'
import { StepCards } from './StepCards'
import { StepExclude } from './StepExclude'

export default function ReportWizard() {
  const { id, step: stepParam } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const isEdit = /\/edit(\/|$)/.test(location.pathname)
  const viewId = isEdit && id ? Number(id) : null
  const step = parseWizardStep(stepParam)
  const { views, loaded, create, saveConfig } = useViews()
  const view = viewId != null ? views.find((v) => v.id === viewId) : undefined

  const [draft, setDraftState] = useState<ReportDraft | null>(() => {
    const existing = loadReportDraft(viewId)
    if (existing) return existing
    if (!isEdit) {
      const d = emptyDraft()
      saveReportDraft(null, d)
      return d
    }
    return null
  })
  const [saving, setSaving] = useState(false)
  const savingRef = useRef(false)
  const draftRef = useRef<ReportDraft | null>(null)
  draftRef.current = draft
  const [saveErr, setSaveErr] = useState('')
  const [emptyAck, setEmptyAck] = useState(false)

  useEffect(() => {
    if (draft) return
    if (!isEdit) return
    if (!loaded) return
    if (!view || viewKind(view) !== 'brand') return
    const d = draftFromView(view.name, (view.config || {}) as ViewConfig)
    saveReportDraft(viewId, d)
    setDraftState(d)
  }, [draft, viewId, isEdit, loaded, view])

  const setDraft = (patch: DraftPatch) => {
    setDraftState((prev) => {
      if (!prev) return prev
      const p = typeof patch === 'function' ? patch(prev) : patch
      const next = { ...prev, ...p }
      draftRef.current = next
      saveReportDraft(viewId, next)
      return next
    })
  }

  if (step == null) return <Navigate to={wizardPath(viewId, 'name')} replace />
  if (isEdit && loaded && !view) {
    return (
      <Card>
        <div className="grid h-40 place-items-center text-[13px] text-t2">
          View not found.
          <button type="button" onClick={() => navigate('/analytics')} className="ml-1 font-semibold text-acc">Go to Main Sales Report</button>
        </div>
      </Card>
    )
  }
  if (isEdit && loaded && view && viewKind(view) === 'model') {
    return <Navigate to={`/models/view/${viewId}`} replace />
  }
  if (!draft) {
    return <div className="grid h-40 place-items-center text-[13px] text-t2">Loading…</div>
  }

  const meta = STEP_META.find((s) => s.id === step)!
  const back = prevStep(step)
  const next = nextStep(step)
  const named = draft.name.trim() !== ''
  const emptyCards = !hasCards(draft)

  const go = (s: WizardStep) => navigate(wizardPath(viewId, s))
  const cancel = () => {
    clearReportDraft(viewId)
    navigate(viewId != null ? `/analytics/view/${viewId}` : '/analytics')
  }
  const continueNext = () => {
    if (step === 'name' && !named) return
    if (step === 'cards' && emptyCards && !emptyAck) { setEmptyAck(true); return }
    if (next) go(next)
  }
  const finish = async () => {
    if (savingRef.current) return
    if (!named) { go('name'); return }
    savingRef.current = true
    setSaving(true)
    setSaveErr('')
    try {
      const cfg = configFromDraft(draftRef.current || draft)
      if (viewId != null) {
        await saveConfig(viewId, cfg, draft.name.trim())
        clearReportDraft(viewId)
        navigate(`/analytics/view/${viewId}`)
      } else {
        const v = await create(draft.name.trim(), cfg)
        clearReportDraft(null)
        navigate(`/analytics/view/${v.id}`)
      }
    } catch (e: any) {
      savingRef.current = false
      setSaveErr(e?.message || 'Could not save the report.')
      setSaving(false)
    }
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (saving) return
    if (next) continueNext()
    else finish()
  }

  return (
    <form onSubmit={onSubmit} className="mx-auto max-w-[720px] space-y-6" noValidate>
      <div>
        <p className="text-[12px] font-bold uppercase tracking-wide text-t2">
          {isEdit ? 'Edit report' : 'New report'}
        </p>
        <h1 className="mt-1 text-xl font-extrabold text-t0 sm:text-[26px]">{meta.title}</h1>
        <p className="mt-1 text-[13px] text-t1">{meta.subtitle}</p>
      </div>

      <ol className="flex items-center gap-1">
        {STEP_META.map((s, i) => {
          const current = s.id === step
          const done = STEP_META.findIndex((x) => x.id === step) > i
          return (
            <li key={s.id} className="flex min-w-0 flex-1 items-center gap-1">
              {i > 0 && <div className={'h-px flex-1 ' + (done || current ? 'bg-acc' : 'bg-line')} />}
              <button
                type="button"
                onClick={() => go(s.id)}
                className="flex min-w-0 items-center gap-2 rounded-[9px] px-1 py-1 text-left hover:bg-bg-3"
              >
                <span className={cn(
                  'grid h-7 w-7 shrink-0 place-items-center rounded-full text-[12px] font-extrabold',
                  current ? 'bg-acc text-white' : done ? 'bg-acc-soft text-acc' : 'bg-bg-3 text-t2',
                )}>
                  {i + 1}
                </span>
                <span className={cn(
                  'hidden truncate text-[12.5px] font-semibold sm:inline',
                  current ? 'text-t0' : 'text-t1',
                )}>
                  {s.label}
                </span>
              </button>
            </li>
          )
        })}
      </ol>

      {step === 'name' && (
        <Card>
          <StepName draft={draft} onChange={(name) => setDraft({ name })} />
        </Card>
      )}
      {step === 'source' && <StepSource draft={draft} onPatch={setDraft} />}
      {step === 'cards' && <StepCards draft={draft} onPatch={setDraft} warnEmpty={emptyAck} />}
      {step === 'exclude' && <StepExclude draft={draft} onPatch={setDraft} />}

      {saveErr && <p className="text-[13px] font-semibold text-bad">{saveErr}</p>}

      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button type="button" variant="ghost" onClick={cancel} disabled={saving}>Cancel</Button>
        <div className="flex flex-wrap items-center gap-2">
          {back && (
            <Button type="button" variant="secondary" onClick={() => go(back)} disabled={saving}>Back</Button>
          )}
          {next ? (
            <Button type="submit" disabled={saving || (step === 'name' && !named)}>
              {step === 'cards' && emptyCards && emptyAck ? 'Continue anyway' : 'Continue'}
            </Button>
          ) : (
            <Button type="submit" disabled={saving || !named}>
              {saving ? 'Saving…' : isEdit ? 'Save report' : 'Create report'}
            </Button>
          )}
        </div>
      </div>
    </form>
  )
}
