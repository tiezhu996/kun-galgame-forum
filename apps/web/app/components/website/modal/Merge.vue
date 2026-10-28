<script setup lang="ts">
import { settle } from '#shared/utils/api/problem'
import type {
  Website,
  WebsiteMergePreview,
  WebsiteSummary
} from '#shared/utils/api/schemas'

const props = defineProps<{
  modelValue: boolean
  website: Website
}>()

const emits = defineEmits<{
  'update:modelValue': [value: boolean]
  merged: [targetHost: string]
}>()

const api = useApiClient()
const idemKey = useIdempotencyKey()

const isModalOpen = computed({
  get: () => props.modelValue,
  set: (value) => emits('update:modelValue', value)
})

const step = ref<'select' | 'preview'>('select')
const sites = ref<WebsiteSummary[]>([])
const keyword = ref('')
const other = ref<WebsiteSummary | null>(null)
const keep = ref<'current' | 'other'>('current')
const listLoading = ref(false)
const previewLoading = ref(false)
const executing = ref(false)
const preview = ref<WebsiteMergePreview | null>(null)

const reset = () => {
  step.value = 'select'
  sites.value = []
  keyword.value = ''
  other.value = null
  keep.value = 'current'
  preview.value = null
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      reset()
      void loadSites()
    }
  }
)

const loadSites = async () => {
  listLoading.value = true
  const collected: WebsiteSummary[] = []
  let cursor: string | undefined
  for (let page = 0; page < 6; page += 1) {
    const result = await settle(
      api.GET('/websites', {
        params: {
          query: {
            limit: 100,
            include_nsfw: true,
            ...(cursor ? { cursor } : {})
          }
        }
      })
    )
    if (!result.ok) {
      reportProblem(result.problem)
      break
    }
    collected.push(...result.data.items)
    cursor = result.data.next_cursor ?? undefined
    if (!cursor) {
      break
    }
  }
  sites.value = collected
  listLoading.value = false
}

const filteredSites = computed(() => {
  const word = keyword.value.trim().toLowerCase()
  return sites.value.filter((site) => {
    if (site.id === props.website.id) {
      return false
    }
    if (!word) {
      return true
    }
    return (
      site.host.toLowerCase().includes(word) ||
      site.title.toLowerCase().includes(word)
    )
  })
})

const pair = computed(() => {
  const current = {
    id: props.website.id,
    host: props.website.host,
    title: props.website.title
  }
  const picked = other.value
    ? {
        id: other.value.id,
        host: other.value.host,
        title: other.value.title
      }
    : null
  return keep.value === 'current'
    ? { source: picked, target: current }
    : { source: current, target: picked }
})

const tagLabel = (tag: { label?: string; slug?: string }) =>
  tag.label || tag.slug || ''

const loadPreview = async () => {
  const { source, target } = pair.value
  if (!source || !target) {
    return
  }
  previewLoading.value = true
  const result = await settle(
    api.POST('/admin/website-merges/preview', {
      body: {
        source_website_id: source.id,
        target_website_id: target.id
      }
    })
  )
  previewLoading.value = false
  if (!result.ok) {
    reportProblem(result.problem)
    return
  }
  preview.value = result.data
  step.value = 'preview'
}

const handleConfirm = async () => {
  const { source, target } = pair.value
  if (!source || !target || executing.value) {
    return
  }
  const payload = {
    source_website_id: source.id,
    target_website_id: target.id
  }
  const confirmed = await useComponentMessageStore().alert(
    `确认将 ${source.host} 合并到 ${target.host} 吗？`,
    '旧域名将永久 301 跳转到新页面，点赞收藏只计一次，该操作不可撤销'
  )
  if (!confirmed) {
    return
  }
  executing.value = true
  const result = await settle(
    api.POST('/admin/website-merges', {
      params: {
        header: {
          'Idempotency-Key': idemKey.take('/admin/website-merges', payload)
        }
      },
      body: payload
    })
  )
  executing.value = false
  if (!result.ok) {
    reportProblem(result.problem)
    return
  }
  useMessage('网站合并成功', 'success')
  isModalOpen.value = false
  emits('merged', result.data.target_host)
}

const busy = computed(() => listLoading.value || previewLoading.value || executing.value)
</script>

<template>
  <KunModal
    :is-dismissable="false"
    v-model="isModalOpen"
    aria-label="合并重复网站"
    inner-class-name="max-w-2xl"
  >
    <div>
      <h2 class="mb-1 text-xl font-bold">合并重复网站</h2>
      <p class="text-default-500 mb-4 text-sm">
        当前站点：{{ website.title }} ({{ website.host }})
      </p>

      <div v-if="step === 'select'" class="space-y-4">
        <KunInput
          v-model="keyword"
          label="搜索另一条站点记录"
          placeholder="输入域名或标题关键字"
        />

        <div>
          <label class="mb-1 block text-sm font-medium">选择重复站点</label>
          <div
            class="border-default-200 max-h-56 space-y-1 overflow-y-auto rounded-lg border p-2"
          >
            <div
              v-if="listLoading"
              class="text-default-500 p-2 text-center text-sm"
            >
              正在加载站点...
            </div>
            <KunNull
              v-else-if="filteredSites.length === 0"
              description="没有匹配的站点"
            />
            <button
              v-for="site in filteredSites"
              :key="site.id"
              type="button"
              class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left"
              :class="cn(
                'hover:bg-default-100',
                other?.id === site.id
                  ? 'bg-primary-50 ring-primary ring-1'
                  : ''
              )"
              @click="other = site"
            >
              <span>
                <span class="block text-sm font-medium">{{ site.title }}</span>
                <span class="text-default-500 block text-xs">{{
                  site.host
                }}</span>
              </span>
              <KunIcon
                v-if="other?.id === site.id"
                name="lucide:check"
                class="text-primary h-4 w-4"
              />
            </button>
          </div>
        </div>

        <div v-if="other">
          <label class="mb-1 block text-sm font-medium">保留哪一条记录</label>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <button
              type="button"
              class="rounded-lg border px-3 py-2 text-left"
              :class="keep === 'current' ? 'border-primary ring-primary ring-1' : ''"
              @click="keep = 'current'"
            >
              <span class="block text-sm font-medium">保留当前站点</span>
              <span class="text-default-500 block text-xs">{{
                website.host
              }}</span>
            </button>
            <button
              type="button"
              class="rounded-lg border px-3 py-2 text-left"
              :class="keep === 'other' ? 'border-primary ring-primary ring-1' : ''"
              @click="keep = 'other'"
            >
              <span class="block text-sm font-medium">保留选中站点</span>
              <span class="text-default-500 block text-xs">{{
                other.host
              }}</span>
            </button>
          </div>
        </div>
      </div>

      <div v-else-if="preview" class="space-y-4">
        <p class="text-sm">
          将把
          <span class="font-medium">{{
            preview.source_website.host
          }}</span>
          合并到
          <span class="font-medium">{{
            preview.target_website.host
          }}</span>
          ，保留站点的标题、描述、分类、图标保持不变。
        </p>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div class="border-default-200 rounded-lg border p-3">
            <h3 class="mb-2 text-sm font-semibold">点赞</h3>
            <p class="text-default-600 text-sm">
              新增 {{ preview.likes.moved }} · 重复
              {{ preview.likes.overlap }} · 合并后
              {{ preview.likes.resulting }}
              （来源 {{ preview.likes.source_count }}）
            </p>
          </div>
          <div class="border-default-200 rounded-lg border p-3">
            <h3 class="mb-2 text-sm font-semibold">收藏</h3>
            <p class="text-default-600 text-sm">
              新增 {{ preview.favorites.moved }} · 重复
              {{ preview.favorites.overlap }} · 合并后
              {{ preview.favorites.resulting }}
              （来源 {{ preview.favorites.source_count }}）
            </p>
          </div>
          <div class="border-default-200 rounded-lg border p-3">
            <h3 class="mb-2 text-sm font-semibold">标签</h3>
            <p class="text-default-600 text-sm">
              新增 {{ preview.tags_added.length }}
              <template v-if="preview.tags_added.length">
                （{{ preview.tags_added.map(tagLabel).join('、') }}）
              </template>
            </p>
            <p
              v-if="preview.tags_dropped.length"
              class="text-default-500 mt-1 text-xs"
            >
              不并入
              {{
                preview.tags_dropped
                  .map(
                    (drop) =>
                      `${tagLabel(drop.tag)}${
                        drop.drop_reason === 'single_select_group'
                          ? '（单选组已保留其他标签）'
                          : '（重复）'
                      }`
                  )
                  .join('、')
              }}
            </p>
          </div>
          <div class="border-default-200 rounded-lg border p-3">
            <h3 class="mb-2 text-sm font-semibold">评论与访问</h3>
            <p class="text-default-600 text-sm">
              评论合并后 {{ preview.comments_resulting }} · 访问量
              {{ preview.view_resulting }}
            </p>
            <p
              v-if="preview.urls_added.length"
              class="text-default-500 mt-1 text-xs"
            >
              并入备用域名 {{ preview.urls_added.length }} 个
            </p>
          </div>
        </div>

        <div
          v-if="preview.is_target_upgraded_to_nsfw"
          class="rounded-lg border border-warning-300 bg-warning-50 px-3 py-2 text-sm text-warning-700"
        >
          保留站点将被标记为 NSFW（被合并站点为 18 禁）
        </div>

        <div
          class="rounded-lg border border-danger-300 bg-danger-50 px-3 py-2 text-sm text-danger-700"
        >
          旧域名将永久 301 跳转到保留站点，评论将在保留页合并展示，此操作不可撤销
        </div>
      </div>

      <div class="mt-6 flex justify-end gap-3">
        <KunButton
          variant="light"
          :disabled="busy"
          @click="isModalOpen = false"
        >
          取消
        </KunButton>
        <KunButton
          v-if="step === 'preview'"
          variant="light"
          :loading="previewLoading"
          @click="step = 'select'"
        >
          上一步
        </KunButton>
        <KunButton
          v-if="step === 'select'"
          color="primary"
          :disabled="!other || listLoading"
          :loading="previewLoading"
          @click="loadPreview"
        >
          预览合并
        </KunButton>
        <KunButton
          v-else
          color="danger"
          :loading="executing"
          @click="handleConfirm"
        >
          确认合并
        </KunButton>
      </div>
    </div>
  </KunModal>
</template>
