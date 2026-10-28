import { useDebounceFn } from '@vueuse/core'
import { useTopicDraft } from './useTopicDraft'
import type { DraftSaveResult } from './useTopicDraft'
import type { TopicDraft } from '#shared/utils/api/schemas'

export type DraftAutosaveStatus =
  | 'idle'
  | 'saving'
  | 'saved'
  | 'conflict'
  | 'error'

const AUTOSAVE_DEBOUNCE_MS = 8000
const CHANNEL_NAME = 'kungal-topic-draft'

// Module-wide singleton: the topbar indicator, the draft modal and the
// editor lifecycle all read one save state instead of each running their own.
const status = ref<DraftAutosaveStatus>('idle')
const lastSavedAt = ref<number | null>(null)
const conflictServerDraft = ref<TopicDraft | null>(null)

// Shared across editor-mount cycles (route round-trips), so a window opened
// on another editor tab still adopts a draft this tab binds after remounting.
let channel: BroadcastChannel | null = null
let lastSavedSignature = ''

export const useTopicDraftAutosave = () => {
  const {
    draftId,
    draftRevision,
    buildDraftBody,
    saveDraft,
    applyDraft,
    hasCurrentContent
  } = useTopicDraft()
  const tempStore = useTempEditStore()

  const signature = () => JSON.stringify(buildDraftBody())

  const post = (
    message:
      | { type: 'bound'; draftId: string; revision: number; signature: string }
      | { type: 'saved'; draftId: string; revision: number; signature: string }
  ) => {
    channel?.postMessage(message)
  }

  const finishSave = async (
    snapshot: boolean,
    force = false
  ): Promise<DraftSaveResult | undefined> => {
    if (tempStore.isTopicRewriting || !hasCurrentContent.value) {
      return undefined
    }
    status.value = 'saving'
    const sentSignature = signature()
    const wasUnbound = !draftId.value
    const result = await saveDraft(snapshot, force)
    if (result.status === 'saved' && result.draft) {
      status.value = 'saved'
      lastSavedAt.value = Date.now()
      lastSavedSignature = sentSignature
      conflictServerDraft.value = null
      post({
        type: wasUnbound ? 'bound' : 'saved',
        draftId: result.draft.id,
        revision: result.draft.revision,
        signature: sentSignature
      })
    } else if (result.status === 'conflict' && result.server) {
      status.value = 'conflict'
      conflictServerDraft.value = result.server
    } else {
      status.value = 'error'
    }
    return result
  }

  const scheduleAutosave = useDebounceFn(() => {
    if (status.value === 'conflict' || status.value === 'saving') {
      return
    }
    if (!tempStore.isTopicRewriting && signature() !== lastSavedSignature) {
      finishSave(false)
    }
  }, AUTOSAVE_DEBOUNCE_MS)

  // An explicit save pins a revision; it runs immediately, debounce or not.
  const manualSave = () => finishSave(true)

  const flushPending = () => {
    if (
      status.value !== 'conflict' &&
      status.value !== 'saving' &&
      !tempStore.isTopicRewriting &&
      hasCurrentContent.value &&
      signature() !== lastSavedSignature
    ) {
      finishSave(false)
    }
  }

  const adoptServerDraft = () => {
    if (!conflictServerDraft.value) {
      return
    }
    applyDraft(conflictServerDraft.value)
    lastSavedSignature = signature()
    conflictServerDraft.value = null
    status.value = 'saved'
    lastSavedAt.value = Date.now()
  }

  const overwriteWithMine = async () => {
    if (!conflictServerDraft.value) {
      return false
    }
    status.value = 'saving'
    const result = await saveDraft(false, true)
    if (result.status === 'saved' && result.draft) {
      conflictServerDraft.value = null
      status.value = 'saved'
      lastSavedAt.value = Date.now()
      lastSavedSignature = signature()
      post({
        type: 'saved',
        draftId: result.draft.id,
        revision: result.draft.revision,
        signature: lastSavedSignature
      })
      return true
    }
    if (result.status === 'conflict' && result.server) {
      // A third write landed while the dialog was open; show the newest one.
      conflictServerDraft.value = result.server
      status.value = 'conflict'
    } else {
      status.value = 'error'
    }
    return false
  }

  // The editor was just filled from a server draft (load or restore), so the
  // buffer equals the server: do not let the next debounced tick write the
  // same bytes back and bump the revision for nothing.
  const markInSync = () => {
    lastSavedSignature = signature()
    conflictServerDraft.value = null
    status.value = 'saved'
    lastSavedAt.value = Date.now()
  }

  // Owned by the editor layout's lifecycle: stop watching once it unmounts,
  // then re-arm when the editor page is mounted again.
  const init = () => {
    if (typeof BroadcastChannel !== 'undefined' && !channel) {
      channel = new BroadcastChannel(CHANNEL_NAME)
      channel.onmessage = (event: MessageEvent) => {
        const msg = event.data as {
          type: string
          draftId: string
          revision: number
          signature: string
        }
        if (!msg || typeof msg.draftId !== 'string') {
          return
        }
        if (msg.type === 'bound' && !draftId.value) {
          // Another tab created the draft for this exact buffer: adopt it
          // instead of minting a second draft ten seconds later.
          if (msg.signature === signature()) {
            draftId.value = msg.draftId
            draftRevision.value = msg.revision
            lastSavedSignature = msg.signature
            status.value = 'saved'
            lastSavedAt.value = Date.now()
          }
          return
        }
        if (msg.type === 'saved' && msg.draftId === draftId.value) {
          if (msg.signature === signature()) {
            draftRevision.value = msg.revision
            lastSavedSignature = msg.signature
            status.value = 'saved'
            lastSavedAt.value = Date.now()
          }
        }
      }
    }

    const stopWatch = watch(
      () => signature(),
      () => {
        if (!tempStore.isTopicRewriting) {
          scheduleAutosave()
        }
      }
    )

    const onHidden = () => {
      if (document.visibilityState === 'hidden') {
        flushPending()
      }
    }
    document.addEventListener('visibilitychange', onHidden)
    onBeforeUnmount(() => {
      flushPending()
      stopWatch()
      document.removeEventListener('visibilitychange', onHidden)
      channel?.close()
      channel = null
    })
  }

  // True while the editor buffer differs from its last saved content, so the
  // modal can warn before a load/restore overwrites unsaved work.
  const hasUnsavedChanges = () =>
    hasCurrentContent.value && signature() !== lastSavedSignature

  return {
    status,
    lastSavedAt,
    conflictServerDraft,
    init,
    manualSave,
    adoptServerDraft,
    overwriteWithMine,
    markInSync,
    hasUnsavedChanges
  }
}
