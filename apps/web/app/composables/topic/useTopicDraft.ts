import { storeToRefs } from 'pinia'
import { useTopicEditorStore } from './useTopicEditorStore'
import { coverHashFromToken, coverTokenFromHash } from './applyTopicSource'
import { useIdempotencyKey } from '~/composables/useIdempotencyKey'
import {
  lotteryFormFromDraft,
  lotteryFormToDraft,
  lotteryHasCodes,
  pollFormFromDraft,
  pollFormToDraft
} from './pendingMiniApps'
import { settle } from '#shared/utils/api/problem'
import type {
  TopicDraft,
  TopicDraftCreate,
  TopicDraftRevision,
  TopicDraftRevisionSummary,
  TopicDraftSummary,
  TopicDraftUpdate
} from '#shared/utils/api/schemas'
import type { DraftDiffSource } from '~/utils/topicDraftDiff'

export type DraftSaveStatus = 'saved' | 'conflict' | 'failed'

export interface DraftSaveResult {
  status: DraftSaveStatus
  draft?: TopicDraft
  server?: TopicDraft
}

export const useTopicDraft = () => {
  const { title, content, section, isNSFW, coverImages, polls, lotteries } =
    useTopicEditorStore()
  const persistStore = usePersistEditTopicStore()
  const { draftId, draftRevision } = storeToRefs(persistStore)
  const api = useApiClient()
  const createKey = useIdempotencyKey()

  const buildDraftBody = (): TopicDraftCreate => ({
    title: title.value,
    content_markdown: content.value,
    sections: section.value as TopicDraftCreate['sections'],
    is_nsfw: isNSFW.value,
    cover_image_hashes: coverImages.value.map(coverHashFromToken),
    polls: polls.value.map(pollFormToDraft),
    lotteries: lotteries.value.map(lotteryFormToDraft)
  })

  const applyDraft = (draft: TopicDraft) => {
    title.value = draft.title
    content.value = draft.content_markdown
    section.value = draft.sections.slice(0, 1)
    isNSFW.value = draft.is_nsfw
    coverImages.value = draft.cover_image_hashes.map(coverTokenFromHash)
    polls.value = draft.polls.map(pollFormFromDraft)
    lotteries.value = draft.lotteries.map(lotteryFormFromDraft)
    draftId.value = draft.id
    draftRevision.value = draft.revision
  }

  const clearBinding = () => {
    draftId.value = ''
    draftRevision.value = 0
  }

  const listDrafts = async (): Promise<TopicDraftSummary[]> => {
    const drafts: TopicDraftSummary[] = []
    let cursor: string | undefined
    do {
      const page = await settle(
        api.GET('/me/topic-drafts', {
          params: { query: { limit: 30, cursor } }
        })
      )
      if (!page.ok) {
        reportProblem(page.problem)
        return drafts
      }
      drafts.push(...page.data.items)
      cursor = page.data.next_cursor
    } while (cursor)
    return drafts
  }

  const fetchDraft = async (id: string) => {
    const draft = await settle(
      api.GET('/me/topic-drafts/{draft_id}', {
        params: { path: { draft_id: id } }
      })
    )
    if (!draft.ok) {
      reportProblem(draft.problem)
      return null
    }
    return draft.data
  }

  // Continue-save over the bound draft. snapshot=true is the author's manual
  // save and pins a revision; false is silent autosave.
  const patchDraft = async (
    snapshot: boolean,
    force = false
  ): Promise<DraftSaveResult> => {
    const body: TopicDraftUpdate = {
      ...buildDraftBody(),
      base_revision: draftRevision.value,
      is_manual_save: snapshot,
      is_force_overwrite: force
    }
    const saved = await settle(
      api.PATCH('/me/topic-drafts/{draft_id}', {
        params: { path: { draft_id: draftId.value } },
        body
      })
    )
    if (saved.ok) {
      draftRevision.value = saved.data.revision
      return { status: 'saved', draft: saved.data }
    }
    if (saved.problem.code === 'DRAFT_REVISION_CONFLICT') {
      const server = await fetchDraft(draftId.value)
      return server ? { status: 'conflict', server } : { status: 'failed' }
    }
    reportProblem(saved.problem)
    return { status: 'failed' }
  }

  const createDraft = async (): Promise<DraftSaveResult> => {
    const body = buildDraftBody()
    const created = await settle(
      api.POST('/me/topic-drafts', {
        params: {
          header: {
            'Idempotency-Key': createKey.take('/me/topic-drafts', body)
          }
        },
        body
      })
    )
    if (!created.ok) {
      reportProblem(created.problem)
      return { status: 'failed' }
    }
    createKey.clear()
    draftId.value = created.data.id
    draftRevision.value = created.data.revision
    return { status: 'saved', draft: created.data }
  }

  const saveDraft = async (
    snapshot: boolean,
    force = false
  ): Promise<DraftSaveResult> =>
    draftId.value ? patchDraft(snapshot, force) : createDraft()

  const loadDraft = async (id: string) => {
    const draft = await fetchDraft(id)
    if (!draft) {
      return false
    }
    applyDraft(draft)
    return true
  }

  const deleteDraft = async (id: string) => {
    const deleted = await settle(
      api.DELETE('/me/topic-drafts/{draft_id}', {
        params: { path: { draft_id: id } }
      })
    )
    if (!deleted.ok) {
      reportProblem(deleted.problem)
      return false
    }
    if (id === draftId.value) {
      clearBinding()
    }
    return true
  }

  const listRevisions = async (
    id: string
  ): Promise<TopicDraftRevisionSummary[]> => {
    const res = await settle(
      api.GET('/me/topic-drafts/{draft_id}/revisions', {
        params: { path: { draft_id: id } }
      })
    )
    if (!res.ok) {
      reportProblem(res.problem)
      return []
    }
    return res.data.items
  }

  const getRevision = async (
    id: string,
    revisionId: string
  ): Promise<TopicDraftRevision | null> => {
    const res = await settle(
      api.GET('/me/topic-drafts/{draft_id}/revisions/{revision_id}', {
        params: { path: { draft_id: id, revision_id: revisionId } }
      })
    )
    if (!res.ok) {
      reportProblem(res.problem)
      return null
    }
    return res.data
  }

  // The server snapshots the current working content first, so restoring a
  // revision can never destroy the editor's current content.
  const restoreRevision = async (
    id: string,
    revisionId: string
  ): Promise<DraftSaveResult> => {
    const res = await settle(
      api.POST('/me/topic-drafts/{draft_id}/revisions/{revision_id}/restore', {
        params: { path: { draft_id: id, revision_id: revisionId } },
        body: { base_revision: draftRevision.value }
      })
    )
    if (res.ok) {
      applyDraft(res.data)
      return { status: 'saved', draft: res.data }
    }
    if (res.problem.code === 'DRAFT_REVISION_CONFLICT') {
      const server = await fetchDraft(id)
      return server ? { status: 'conflict', server } : { status: 'failed' }
    }
    reportProblem(res.problem)
    return { status: 'failed' }
  }

  const currentDraftSource = (): DraftDiffSource => ({
    title: title.value,
    content: content.value,
    sections: section.value,
    is_nsfw: isNSFW.value
  })

  const revisionDiffSource = (
    draft: TopicDraft | TopicDraftRevision
  ): DraftDiffSource => ({
    title: draft.title,
    content: draft.content_markdown,
    sections: draft.sections,
    is_nsfw: draft.is_nsfw
  })

  const hasCurrentContent = computed(
    () => !!(title.value.trim() || content.value.trim())
  )

  const hasMiniApps = computed(
    () => polls.value.length + lotteries.value.length > 0
  )

  const hasUnsavableCodes = computed(() =>
    lotteries.value.some(lotteryHasCodes)
  )

  return {
    draftId,
    draftRevision,
    listDrafts,
    buildDraftBody,
    saveDraft,
    loadDraft,
    deleteDraft,
    applyDraft,
    clearBinding,
    fetchDraft,
    listRevisions,
    getRevision,
    restoreRevision,
    currentDraftSource,
    revisionDiffSource,
    hasCurrentContent,
    hasMiniApps,
    hasUnsavableCodes
  }
}
