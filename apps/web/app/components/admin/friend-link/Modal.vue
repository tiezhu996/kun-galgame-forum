<script setup lang="ts">
import type {
  AdminFriendLink,
  FriendLinkCategory,
  FriendLinkCreate,
  FriendLinkPatch,
  FriendLinkState
} from '#shared/utils/api/schemas'
import {
  FRIEND_LINK_CATEGORY_OPTIONS,
  FRIEND_LINK_STATE_OPTIONS
} from '~/constants/friendLink'
import type { FriendLinkSubmit } from './type'

const props = defineProps<{
  modelValue: boolean
  initialData?: AdminFriendLink | null
  defaultCategory?: FriendLinkCategory
}>()

const emits = defineEmits<{
  'update:modelValue': [value: boolean]
  submit: [data: FriendLinkSubmit]
}>()

const isOpen = computed({
  get: () => props.modelValue,
  set: (v) => emits('update:modelValue', v)
})
const isEditing = computed(() => !!props.initialData?.id)
const isAbnormal = computed(() => props.initialData?.state === 'abnormal')

type FriendLinkForm = Required<
  Omit<FriendLinkCreate, 'banner_image_hash' | 'description' | 'state'>
> & {
  description: string
  banner_image_hash: string
  state: FriendLinkState
}

const normalizeState = (state?: AdminFriendLink['state']): FriendLinkState =>
  state === 'down' ? 'down' : 'normal'

const getInitial = (): FriendLinkForm => {
  const d = props.initialData
  return {
    friend_link_category:
      d?.friend_link_category ?? props.defaultCategory ?? 'galgame',
    title: d?.title ?? '',
    url: d?.url ?? '',
    description: d?.description ?? '',
    banner_image_hash: d?.banner?.hash ?? '',
    state: normalizeState(d?.state)
  }
}

const form = reactive<FriendLinkForm>(getInitial())
watch(
  () => props.modelValue,
  (open) => {
    if (open) Object.assign(form, getInitial())
  }
)

const initialBannerUrl = computed(() => props.initialData?.banner?.url ?? '')

const changedFields = (): FriendLinkPatch => {
  const initial = getInitial()
  const patch: FriendLinkPatch = {}
  for (const key of Object.keys(form) as (keyof FriendLinkForm)[]) {
    if (form[key] !== initial[key]) {
      Object.assign(patch, { [key]: form[key] })
    }
  }
  // abnormal is not a selectable value: saving an abnormal row while the form
  // shows normal is the rescue action, so the state patch must be explicit.
  if (isAbnormal.value && form.state !== props.initialData?.state) {
    patch.state = form.state
  }
  return patch
}

const handleSubmit = () => {
  if (!form.title.trim()) {
    useMessage('请填写友链名称', 'warn')
    return
  }
  if (!/^https?:\/\//.test(form.url.trim())) {
    useMessage('友链地址必须以 http:// 或 https:// 开头', 'warn')
    return
  }
  const id = props.initialData?.id
  if (id) {
    const body = changedFields()
    if (Object.keys(body).length) {
      emits('submit', { id, body })
    }
  } else {
    emits('submit', { id: null, body: { ...form, url: form.url.trim() } })
  }
  isOpen.value = false
}
</script>

<template>
  <KunModal
    :is-dismissable="false"
    v-model="isOpen"
    inner-class-name="max-w-2xl"
  >
    <form @submit.prevent>
      <h2 class="mb-4 text-xl font-bold">
        {{ isEditing ? '编辑友链' : '添加友链' }}
      </h2>

      <div
        v-if="isAbnormal"
        class="bg-warning-50 text-warning-700 mb-4 flex items-start gap-2 rounded-lg p-3 text-sm"
      >
        <KunIcon name="lucide:triangle-alert" class="mt-0.5 size-4 shrink-0" />
        <span>
          该站点已被健康检查连续 3 次判定为打不开并从公开页收起。保存时状态保持「正常」即立即放回公开页；改为「已下线」则保留为手动下线。
        </span>
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <KunInput v-model="form.title" label="名称" required />
        <KunInput
          v-model="form.url"
          label="链接 (URL)"
          required
          placeholder="https://..."
        />
        <KunSelect
          v-model="form.friend_link_category"
          label="分类"
          :options="FRIEND_LINK_CATEGORY_OPTIONS"
        />
        <KunSelect
          v-model="form.state"
          label="状态"
          :options="FRIEND_LINK_STATE_OPTIONS"
        />
        <KunTextarea
          v-model="form.description"
          label="描述"
          auto-grow
          show-char-count
          :maxlength="500"
          class-name="md:col-span-2"
        />

        <div class="md:col-span-2">
          <KunCoverUpload
            v-model="form.banner_image_hash"
            :preview-url="initialBannerUrl"
            label="图标 / Banner"
          />
        </div>
      </div>

      <div class="mt-6 flex justify-end gap-3">
        <KunButton variant="light" color="danger" @click="isOpen = false">
          取消
        </KunButton>
        <KunButton color="primary" @click="handleSubmit">
          {{ isEditing ? '保存' : '添加' }}
        </KunButton>
      </div>
    </form>
  </KunModal>
</template>
