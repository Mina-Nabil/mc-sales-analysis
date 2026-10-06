// Shared car-tree vocabulary and form styling.
//
// These lists were duplicated verbatim in Tree.tsx and Review.tsx; both pages
// create the same nodes, so they must offer the same values. ORIGINS mirrors the
// values history uses in brands.origin — the column is free text, so this list is
// the only thing keeping it tidy.
export const CAR_TYPES = ['Passenger', 'Commercial', 'Bus', 'Construction']
export const ENGINES = ['ICE', 'HYBRID', 'BEV', 'REEV', 'Other']
export const SUPPLIES = ['CKD', 'SUP']
export const ORIGINS = ['China', 'Europe', 'Japan', 'Korea', 'USA', 'India', 'Russia', 'UAE', 'Egypt', 'Others']

export const field =
  'h-9 w-full rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0 focus:border-acc'
