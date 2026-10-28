<script setup lang="ts">
import type { UserSearchHit } from '#shared/utils/api/schemas'
import { useDebounceFn } from '@vueuse/core'
import { settle } from '#shared/utils/api/problem'
import { deletedUserName } from '~/utils/userRef'

const props = defineProps<{
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const api = useApiClient()

const popoverRef = ref<{ close: () => void } | null>(null)
const keyword = ref('')
const pending = ref(false)
const matches = ref<UserSearchHit[]>([])
let latest = 0

const { entry, load } = useUserCard(() => props.modelValue)

watch(
  () => props.modelValue,
  (id) => {
    if (id && entry.value?.state !== 'ready') {
      void load()
    }
  },
  { immediate: true }
)

const selectedUser = computed<KunUser | null>(() => {
  if (!props.modelValue) {
    return null
  }
  if (entry.value?.state === 'ready') {
    const p = entry.value.profile
    return {
      id: Number(p.id),
      name: p.name ?? deletedUserName,
      avatar: p.avatar?.url ?? ''
    }
  }
  return { id: Number(props.modelValue), name: '加载中…', avatar: '' }
})

const search = async (value: string) => {
  const current = ++latest
  const q = value.trim()
  if (!q) {
    matches.value = []
    pending.value = false
    return
  }
  pending.value = true
  const result = await settle(
    api.GET('/search/users', { params: { query: { q, limit: 8 } } })
  )
  if (current !== latest) {
    return
  }
  pending.value = false
  matches.value = result.ok ? result.data.items : []
}

const debouncedSearch = useDebounceFn(search, 300)

watch(keyword, (value) => void debouncedSearch(value))

const pick = (hit: UserSearchHit) => {
  emit('update:modelValue', hit.id)
  popoverRef.value?.close()
}

const clear = () => emit('update:modelValue', '')
</script>

<template>
  <div v-if="selectedUser" class="flex items-center gap-1">
    <KunPopover ref="popoverRef" position="bottom-start">
      <template #trigger>
        <KunChip size="lg" color="primary" class="cursor-pointer gap-1">
          <KunAvatar
            :user="selectedUser"
            size="sm"
            :is-navigation="false"
          />
          <span>{{ selectedUser.name }}</span>
        </KunChip>
      </template>

      <div class="flex w-72 flex-col gap-2 p-2">
        <KunInput
          v-model="keyword"
          size="sm"
          placeholder="输入用户名搜索"
          is-clearable
        />
        <div class="max-h-72 overflow-y-auto">
          <div
            v-if="pending"
            class="text-default-500 px-2 py-3 text-center text-sm"
          >
            搜索中…
          </div>
          <div
            v-else-if="!matches.length"
            class="text-default-500 px-2 py-3 text-center text-sm"
          >
            {{ keyword.trim() ? '没有找到用户' : '输入用户名开始搜索' }}
          </div>
          <button
            v-for="hit in matches"
            :key="hit.id"
            type="button"
            class="hover:bg-primary/10 flex w-full items-center gap-2 rounded-lg p-2 text-left"
            @click="pick(hit)"
          >
            <KunAvatar
              :user="{
                id: Number(hit.id),
                name: hit.name ?? deletedUserName,
                avatar: hit.avatar?.url ?? ''
              }"
              size="sm"
              :is-navigation="false"
            />
            <span class="truncate text-sm">{{ hit.name ?? deletedUserName }}</span>
          </button>
        </div>
      </div>
    </KunPopover>

    <KunButton
      size="sm"
      variant="light"
      :is-icon-only="true"
      aria-label="清除发送人筛选"
      @click="clear"
    >
      <KunIcon name="lucide:x" />
    </KunButton>
  </div>

  <KunPopover v-else ref="popoverRef" position="bottom-start">
    <template #trigger>
      <KunButton size="sm" variant="flat">
        <span class="flex items-center gap-1">
          <KunIcon name="lucide:user-search" />
          按发送人
        </span>
      </KunButton>
    </template>

    <div class="flex w-72 flex-col gap-2 p-2">
      <KunInput
        v-model="keyword"
        size="sm"
        placeholder="输入用户名搜索"
        is-clearable
      />
      <div class="max-h-72 overflow-y-auto">
        <div
          v-if="pending"
          class="text-default-500 px-2 py-3 text-center text-sm"
        >
          搜索中…
        </div>
        <div
          v-else-if="!matches.length"
          class="text-default-500 px-2 py-3 text-center text-sm"
        >
          {{ keyword.trim() ? '没有找到用户' : '输入用户名开始搜索' }}
        </div>
        <button
          v-for="hit in matches"
          :key="hit.id"
          type="button"
          class="hover:bg-primary/10 flex w-full items-center gap-2 rounded-lg p-2 text-left"
          @click="pick(hit)"
        >
          <KunAvatar
            :user="{
              id: Number(hit.id),
              name: hit.name ?? deletedUserName,
              avatar: hit.avatar?.url ?? ''
            }"
            size="sm"
            :is-navigation="false"
          />
          <span class="truncate text-sm">{{ hit.name ?? deletedUserName }}</span>
        </button>
      </div>
    </div>
  </KunPopover>
</template>
