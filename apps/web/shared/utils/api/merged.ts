// ENTITY_MERGED carries the survivor's id as an extension member, which the
// shared problem settle() does not keep; read it off the raw error body.
export const mergedInto = (error: unknown): number | null => {
  if (
    error &&
    typeof error === 'object' &&
    (error as { code?: unknown }).code === 'ENTITY_MERGED'
  ) {
    const id = Number((error as { current_id?: unknown }).current_id)
    return Number.isInteger(id) && id > 0 ? id : null
  }
  return null
}

// A website merge additionally names the survivor's host, the value the
// /website/[domain] page needs to build its 301. object must be "website":
// other families use mergedInto() and name only the id.
export const mergedWebsiteHost = (error: unknown): string | null => {
  if (
    error &&
    typeof error === 'object' &&
    (error as { code?: unknown }).code === 'ENTITY_MERGED' &&
    (error as { object?: unknown }).object === 'website'
  ) {
    const host = (error as { current_host?: unknown }).current_host
    return typeof host === 'string' && host ? host : null
  }
  return null
}
