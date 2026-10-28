<script setup lang="ts">
import { useTopicDraft } from '~/composables/topic/useTopicDraft'
import DraftDiffView from './DraftDiffView.vue'
import type {
  TopicDraftRevision,
  TopicDraftRevisionSummary
} from '#shared/utils/api/schemas'

const open = defineModel<boolean>({ required: true })

const props = defineProps<{
  draftId: string
}>()

const emit = defineEmits<{
  restored: []
}>()

const { listRevisions, getRevision, restoreRevision, revisionDiffSource } =
  useTopicDraft()

const revisions = ref<TopicDraftRevisionSummary[]>([])
const isLoading = ref(false)
const fullCache = ref<Record<string, TopicDraftRevision>>({})
const oldId = ref('')
const newId = ref('')
const isRestoring = ref(false)

const kindLabel = (kind: string) =>
  kind === 'manual' ? '手动保存' : '自动留存'

const fullOf = (id: string) => fullCache.value[id] ?? null

const loadFull = async (id: string) => {
  if (!id || fullCache.value[id]) {
    return
  }
  const rev = await getRevision(props.draftId, id)
  if (rev) {
    fullCache.value = { ...fullCache.value, [id]: rev }
  }
}

const refresh = async () => {
  isLoading.value = true
  revisions.value = await listRevisions(props.draftId)
  isLoading.value = false
  const newest = revisions.value[0]
  if (newest) {
    const previous = revisions.value[1]
    newId.value = newest.id
    oldId.value = previous ? previous.id : newest.id
  }
}

watch(open, (isOpen) => {
  if (isOpen && props.draftId) {
    refresh()
  }
})

watch([oldId, newId], ([a, b]) => {
  if (a) {
    loadFull(a)
  }
  if (b && b !== a) {
    loadFull(b)
  }
})

const onRestore = async (revision: TopicDraftRevisionSummary) => {
  const ok = await useComponentMessageStore().alert(
    '恢复此版本？',
    '当前编辑内容会先自动留存为一个新版本，然后载入所选版本，随时可以再切回来。',
    true
  )
  if (!ok) {
    return
  }
  isRestoring.value = true
  const result = await restoreRevision(props.draftId, revision.id)
  isRestoring.value = false
  if (result.status === 'saved') {
    useMessage('已恢复所选版本，恢复前的内容已自动留存', 'success')
    open.value = false
    emit('restored')
    return
  }
  if (result.status === 'conflict') {
    useMessage(
      '该草稿刚在其他窗口被保存，请先处理保存冲突后再恢复',
      'warn',
      6000
    )
    open.value = false
  }
}

const displayTitle = (revision: TopicDraftRevisionSummary) =>
  revision.title.trim() ||
  truncateRunes(markdownToText(revision.summary), 40) ||
  '(无标题版本)'
</script>

<template>
  <KunModal v-model="open" inner-class-name="max-w-2xl">
    <div class="flex max-h-[85vh] flex-col">
      <header class="mb-4 flex items-center gap-2">
        <KunIcon name="lucide:history" class="text-primary-500 h-5 w-5" />
        <h2 class="text-xl font-bold">版本历史</h2>
      </header>

      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1">
        <div v-if="isLoading" class="text-default-400 py-8 text-center text-sm">
          加载中…
        </div>
        <template v-else>
          <div class="border-default-200 rounded-lg border">
            <div
              v-for="(revision, i) in revisions"
              :key="revision.id"
              class="border-default-200 flex items-center gap-3 p-3"
              :class="i > 0 && 'border-t'"
            >
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span
                    :class="
                      cn(
                        'shrink-0 rounded px-1.5 py-0.5 text-[11px]',
                        revision.kind === 'manual'
                          ? 'bg-primary-500/10 text-primary-600'
                          : 'bg-default-200 text-default-500'
                      )
                    "
                  >
                    {{ kindLabel(revision.kind) }}
                  </span>
                  <span class="truncate text-sm font-medium">
                    {{ displayTitle(revision) }}
                  </span>
                </div>
                <div class="text-default-400 mt-0.5 text-xs">
                  <KunTime :time="revision.created_at" type="datetime" />
                </div>
              </div>
              <KunButton
                size="sm"
                variant="light"
                :disabled="isRestoring || i === 0"
                :title="i === 0 ? '当前已是这个版本' : '恢复此版本'"
                @click="onRestore(revision)"
              >
                恢复
              </KunButton>
            </div>
          </div>

          <div v-if="revisions.length >= 2">
            <div class="mb-2 flex flex-wrap items-center gap-2 text-sm">
              <span class="text-default-500">对比</span>
              <select
                v-model="oldId"
                class="border-default-200 max-w-40 rounded-md border px-2 py-1 text-xs"
              >
                <option
                  v-for="revision in revisions"
                  :key="`a-${revision.id}`"
                  :value="revision.id"
                >
                  {{ kindLabel(revision.kind) }} ·
                  {{ displayTitle(revision) }}
                </option>
              </select>
              <span class="text-default-400">→</span>
              <select
                v-model="newId"
                class="border-default-200 max-w-40 rounded-md border px-2 py-1 text-xs"
              >
                <option
                  v-for="revision in revisions"
                  :key="`b-${revision.id}`"
                  :value="revision.id"
                >
                  {{ kindLabel(revision.kind) }} ·
                  {{ displayTitle(revision) }}
                </option>
              </select>
            </div>
            <DraftDiffView
              v-if="fullOf(oldId) && fullOf(newId)"
              :old-source="revisionDiffSource(fullOf(oldId)!)"
              :new-source="revisionDiffSource(fullOf(newId)!)"
            />
          </div>
        </template>
      </div>
    </div>
  </KunModal>
</template>
