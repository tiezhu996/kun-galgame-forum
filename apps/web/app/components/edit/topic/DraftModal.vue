<script setup lang="ts">
import { useTopicDraft } from '~/composables/topic/useTopicDraft'
import { useTopicDraftAutosave } from '~/composables/topic/useTopicDraftAutosave'
import DraftHistoryModal from './DraftHistoryModal.vue'
import DraftDiffView from './DraftDiffView.vue'
import type { TopicDraftSummary } from '#shared/utils/api/schemas'

const open = defineModel<boolean>({ required: true })

const {
  draftId,
  listDrafts,
  loadDraft,
  deleteDraft,
  currentDraftSource,
  revisionDiffSource,
  hasCurrentContent,
  hasMiniApps,
  hasUnsavableCodes
} = useTopicDraft()

const {
  status,
  lastSavedAt,
  conflictServerDraft,
  manualSave,
  adoptServerDraft,
  overwriteWithMine,
  markInSync,
  hasUnsavedChanges
} = useTopicDraftAutosave()

const drafts = ref<TopicDraftSummary[]>([])
const isLoading = ref(false)
const isSaving = ref(false)
const isResolving = ref(false)
const showConflictDiff = ref(false)
const historyDraftId = ref('')

const refresh = async () => {
  isLoading.value = true
  drafts.value = await listDrafts()
  isLoading.value = false
}

watch(open, (isOpen) => {
  if (isOpen) {
    refresh()
  }
})

const statusText = computed(() => {
  if (status.value === 'saving') {
    return '正在保存…'
  }
  if (status.value === 'conflict') {
    return '保存冲突'
  }
  if (status.value === 'error') {
    return '保存失败'
  }
  if (lastSavedAt.value) {
    return `已自动保存 ${new Date(lastSavedAt.value).toLocaleTimeString(
      'zh-CN',
      {
        hour: '2-digit',
        minute: '2-digit'
      }
    )}`
  }
  return '编辑后会自动保存'
})

const onManualSave = async () => {
  if (isSaving.value || status.value === 'conflict') {
    return
  }
  if (!hasCurrentContent.value) {
    useMessage('当前没有可保存的内容', 'warn')
    return
  }
  isSaving.value = true
  const result = await manualSave()
  isSaving.value = false
  if (result === undefined || result.status !== 'saved') {
    if (result?.status === 'conflict') {
      useMessage('该草稿已在其他窗口被保存，请先处理冲突', 'warn', 6000)
    }
    return
  }
  if (hasUnsavableCodes.value) {
    useMessage(
      '已保存为新版本。抽奖的兑换码不会存进草稿, 载入后需要重新填写',
      'info',
      6000
    )
  } else {
    useMessage('已保存为新版本', 'success')
  }
  refresh()
}

const onLoad = async (draft: TopicDraftSummary) => {
  if (draft.id === draftId.value) {
    open.value = false
    return
  }
  if (hasCurrentContent.value || hasMiniApps.value || hasUnsavedChanges()) {
    const ok = await useComponentMessageStore().alert(
      '载入另一份草稿？',
      '载入会覆盖当前正在编写的内容 (包括已添加的投票与抽奖)，当前未保存的修改将丢失。',
      true
    )
    if (!ok) {
      return
    }
  }
  if (await loadDraft(draft.id)) {
    useMessage('草稿已载入，之后的修改会自动续存到这一份', 'success')
    showConflictDiff.value = false
    markInSync()
    open.value = false
  }
}

const onDelete = async (draft: TopicDraftSummary) => {
  const ok = await useComponentMessageStore().alert(
    '删除这份草稿？',
    '它的全部历史版本也会一起删除，且无法恢复。',
    true
  )
  if (!ok) {
    return
  }
  if (!(await deleteDraft(draft.id))) {
    return
  }
  useMessage('草稿已删除', 'success')
  refresh()
}

const onAdoptServer = async () => {
  const ok = await useComponentMessageStore().alert(
    '采用其他窗口的版本？',
    '当前窗口未保存的修改会被覆盖；对方的内容已在服务器上，无需额外留存。',
    true
  )
  if (!ok) {
    return
  }
  isResolving.value = true
  adoptServerDraft()
  isResolving.value = false
  showConflictDiff.value = false
  useMessage('已载入服务器上的最新版本', 'success')
}

const onOverwrite = async () => {
  const ok = await useComponentMessageStore().alert(
    '用当前内容覆盖？',
    '其他窗口保存的内容会先自动留存为一个版本，之后随时可以恢复。',
    true
  )
  if (!ok) {
    return
  }
  isResolving.value = true
  const won = await overwriteWithMine()
  isResolving.value = false
  if (won) {
    showConflictDiff.value = false
    useMessage('已覆盖，对方的内容已自动留存为版本', 'success')
    refresh()
  }
}

const displayTitle = (draft: TopicDraftSummary) =>
  draft.title.trim() ||
  truncateRunes(markdownToText(draft.summary), 40) ||
  '(无标题草稿)'

const openHistory = (draft: TopicDraftSummary) => {
  historyDraftId.value = draft.id
}

const isHistoryOpen = computed({
  get: () => historyDraftId.value !== '',
  set: (value: boolean) => {
    if (!value) {
      historyDraftId.value = ''
    }
  }
})
</script>

<template>
  <KunModal v-model="open" inner-class-name="max-w-lg">
    <div class="flex max-h-[82vh] flex-col">
      <header class="mb-4 flex items-center gap-2">
        <KunIcon name="lucide:notebook-pen" class="text-primary-500 h-5 w-5" />
        <h2 class="text-xl font-bold">话题草稿</h2>
      </header>

      <div
        v-if="status === 'conflict' && conflictServerDraft"
        class="border-warning-500/40 bg-warning-500/10 mb-4 space-y-2 rounded-lg border p-3 text-sm"
      >
        <div class="flex items-center gap-2 font-medium">
          <KunIcon name="lucide:triangle-alert" class="size-4" />
          这份草稿刚在其他窗口或设备被保存
        </div>
        <p class="text-default-600 text-xs leading-relaxed">
          为避免覆盖对方的内容，自动保存已暂停。请对比后选择：采用对方版本，或用当前内容覆盖（对方内容会自动留存为版本）。
        </p>
        <DraftDiffView
          v-if="showConflictDiff"
          :old-source="currentDraftSource()"
          :new-source="revisionDiffSource(conflictServerDraft)"
        />
        <div class="flex flex-wrap gap-2 pt-1">
          <KunButton
            size="sm"
            variant="light"
            @click="showConflictDiff = !showConflictDiff"
          >
            {{ showConflictDiff ? '收起差异' : '查看差异' }}
          </KunButton>
          <KunButton
            size="sm"
            variant="light"
            :disabled="isResolving"
            @click="onAdoptServer"
          >
            采用对方版本
          </KunButton>
          <KunButton
            size="sm"
            color="primary"
            :disabled="isResolving"
            @click="onOverwrite"
          >
            用我的内容覆盖
          </KunButton>
        </div>
      </div>

      <div class="mb-3 flex items-center justify-between gap-3">
        <KunButton
          color="primary"
          :disabled="isSaving || status === 'conflict'"
          class="shrink-0"
          @click="onManualSave"
        >
          <KunIcon
            v-if="isSaving"
            name="lucide:loader-2"
            class="mr-1 h-4 w-4 animate-spin"
          />
          <KunIcon v-else name="lucide:save" class="mr-1 h-4 w-4" />
          {{ draftId ? '保存新版本' : '保存为草稿' }}
        </KunButton>
        <span
          :class="
            cn(
              'flex items-center gap-1 text-xs',
              status === 'conflict'
                ? 'text-warning-600'
                : status === 'error'
                  ? 'text-danger-500'
                  : 'text-default-400'
            )
          "
        >
          <KunIcon
            :name="
              status === 'conflict' ? 'lucide:triangle-alert' : 'lucide:cloud'
            "
            class="size-3.5"
          />
          {{ statusText }}
        </span>
      </div>

      <div class="text-default-500 mb-2 text-sm">
        编辑会自动续存到当前草稿；「保存新版本」会留下一个可对比、可恢复的版本。每个账号最多
        30 份草稿，每份草稿最多保留 50 个版本，更早的版本会自动清理。
      </div>

      <div class="min-h-0 flex-1 space-y-2 overflow-y-auto">
        <div v-if="isLoading" class="text-default-400 py-8 text-center text-sm">
          加载中…
        </div>
        <div
          v-else-if="!drafts.length"
          class="text-default-400 py-8 text-center text-sm"
        >
          还没有草稿
        </div>
        <template v-else>
          <div
            v-for="draft in drafts"
            :key="draft.id"
            class="border-default-200 hover:border-primary-500 flex items-center gap-3 rounded-lg border p-3 transition-colors"
            :class="
              draft.id === draftId && 'border-primary-500/60 bg-primary-500/5'
            "
          >
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span
                  v-if="draft.id === draftId"
                  class="bg-primary-500/10 text-primary-600 shrink-0 rounded px-1.5 py-0.5 text-[11px]"
                >
                  正在编辑
                </span>
                <span class="truncate font-medium">
                  {{ displayTitle(draft) }}
                </span>
              </div>
              <div class="text-default-400 text-xs">
                <KunTime :time="draft.updated_at" type="datetime" />
                · v{{ draft.revision }}
              </div>
            </div>
            <KunButton size="sm" variant="light" @click="onLoad(draft)">
              {{ draft.id === draftId ? '继续编辑' : '载入' }}
            </KunButton>
            <KunButton size="sm" variant="light" @click="openHistory(draft)">
              版本
            </KunButton>
            <KunButton
              size="sm"
              variant="light"
              color="danger"
              :is-icon-only="true"
              @click="onDelete(draft)"
            >
              <KunIcon name="lucide:trash-2" />
            </KunButton>
          </div>
        </template>
      </div>
    </div>

    <DraftHistoryModal
      v-model="isHistoryOpen"
      :draft-id="historyDraftId"
      @restored="
        () => {
          markInSync()
          refresh()
        }
      "
    />
  </KunModal>
</template>
