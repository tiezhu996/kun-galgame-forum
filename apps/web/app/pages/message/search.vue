<script setup lang="ts">
import type { MessageSearchWithin } from '#shared/utils/api/schemas'

definePageMeta({
  middleware: 'auth',
  pageTransition: false
})

useKunDisableSeo('搜索消息')

const route = useRoute()
const router = useRouter()

const isWithin = (value: unknown): value is MessageSearchWithin =>
  value === 'all' || value === '7d' || value === '30d'

const q = computed(() => {
  const value = route.query.q
  return typeof value === 'string' ? value : ''
})
const senderId = computed(() => {
  const value = route.query.sender_id
  return typeof value === 'string' && /^[1-9][0-9]{0,18}$/.test(value)
    ? value
    : ''
})
const within = computed<MessageSearchWithin>(() =>
  isWithin(route.query.within) ? route.query.within : 'all'
)
const hasQuery = computed(() => q.value.trim().length > 0)

const applySearch = (payload: {
  q: string
  senderId: string
  within: MessageSearchWithin
}) => {
  void router.replace({
    query: {
      ...(payload.q ? { q: payload.q } : {}),
      ...(payload.senderId ? { sender_id: payload.senderId } : {}),
      ...(payload.within !== 'all' ? { within: payload.within } : {})
    }
  })
}
</script>

<template>
  <div class="flex h-full min-h-0 w-full flex-col gap-3 pl-3">
    <header class="flex items-center gap-2">
      <KunButton
        size="lg"
        :is-icon-only="true"
        variant="light"
        href="/message"
      >
        <KunIcon name="lucide:chevron-left" />
      </KunButton>
      <h2 class="text-lg">搜索消息</h2>
    </header>

    <MessageSearchBar
      :q="q"
      :sender-id="senderId"
      :within="within"
      @search="applySearch"
      @update:sender-id="
        (value: string) =>
          applySearch({
            q: q.trim(),
            senderId: value,
            within
          })
      "
    />

    <KunDivider />

    <MessageSearchResultList
      v-if="hasQuery"
      :key="`${q}|${senderId}|${within}`"
      :q="q"
      :sender-id="senderId"
      :within="within"
    />

    <div v-else class="text-default-500 flex flex-1 items-center justify-center text-sm">
      <div class="text-center">
        <KunIcon name="lucide:search" class="mx-auto mb-2 size-8" />
        <p>输入关键词，同时搜索你的通知和私聊</p>
        <p class="mt-1">可按发送人和最近 7 天 / 30 天缩小范围</p>
      </div>
    </div>
  </div>
</template>
