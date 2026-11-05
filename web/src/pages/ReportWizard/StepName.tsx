import { Input, FormField } from '@/components/ui'
import type { ReportDraft } from '@/lib/reportDraft'

export function StepName({ draft, onChange }: { draft: ReportDraft; onChange: (name: string) => void }) {
  return (
    <FormField label="Report name" required hint="Shown in the sidebar under User Reports.">
      <Input
        autoFocus
        placeholder="e.g. China SUVs — 2026"
        value={draft.name}
        onChange={(e) => onChange(e.target.value)}
      />
    </FormField>
  )
}
