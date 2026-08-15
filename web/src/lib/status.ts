export type StatusVariant = "success" | "warning" | "danger" | "info" | "accent" | "neutral";

const SUCCESS = ["active", "completed", "paid", "delivered", "approved", "success", "in stock", "won", "resolved", "verified", "published", "online", "confirmed", "shipped"];
const WARNING = ["pending", "in progress", "processing", "on hold", "low stock", "review", "draft", "trial", "warning", "scheduled", "waiting"];
const DANGER = ["inactive", "cancelled", "canceled", "failed", "rejected", "overdue", "out of stock", "lost", "error", "expired", "blocked", "urgent", "high"];
const INFO = ["new", "info", "open", "invited", "queued", "medium"];

/** Maps a free-text status/severity label to a badge color variant, so
 * mock data can stay plain strings instead of baking colors into records. */
export function statusVariant(status: string): StatusVariant {
  const s = status.trim().toLowerCase();
  if (SUCCESS.includes(s)) return "success";
  if (WARNING.includes(s)) return "warning";
  if (DANGER.includes(s)) return "danger";
  if (INFO.includes(s)) return "info";
  return "neutral";
}
