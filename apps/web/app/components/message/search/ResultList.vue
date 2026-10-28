<script setup lang="ts">
import type {
  MessageSearchResult,
  MessageSearchWithin
} from '#shared/utils/api/schemas'
import { problemMessage } from '#shared/utils/api/message'
import { useCursorList } from '~/composables/useCursorList'

const props = defineProps<{
  q: string
  senderId: string
  within: MessageSearchWithin
}>()

const {
  items,
  hasMore,
  problem,
  pending,
  loadingMore,
  loadMore,
  refresh
} = useCursorList<MessageSearchResult>(
  () =>
    `me-message-search:${props.q}:${props.senderId}:${props.within}`,
  (api, cursor, { signal }) =>
    api.GET('/me/message-search', {
      params: {
        query: {
          q: props.q,
          limit: 20,
          ...(props.senderId ? { sender_id: props.senderId } : {}),
          ...(props.within !== 'all' ? { within: props.within } : {}),
          ...(cursor ? { cursor } : {})
        }
      },
      signal
    })
)
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <template v-if="pending">
      <div class="space-y-3">
        <div
          v-for="i in 5"
          :key="i"
          class="border-default-200/60 h-24 animate-pulse rounded-lg border"
        />
      </div>
    </template>

    <template v-else-if="problem">
      <KunNull :description="problemMessage(problem)" />
      <div class="flex justify-center pt-2">
        <KunButton variant="flat" size="sm" @click="() => refresh()">
          重试
        </KunButton>
      </div>
    </template>

    <template v-else-if="items.length">
      <KunOverlayScroll class="min-h-0 flex-1">
        <div class="space-y-3 py-1">
          <MessageSearchResultItem
            v-for="result in items"
            :key="result.id"
            :result="result"
            :phrase="q"
          />
        </div>
        <div v-if="hasMore" class="flex justify-center pt-3 pb-2">
          <KunButton
            variant="light"
            :loading="loadingMore"
            @click="loadMore"
          >
            加载更多
          </KunButton>
        </div>
      </KunOverlayScroll>
    </template>

    <KunNull v-else description="没有匹配的消息，换个关键词试试" />
  </div>
</template>
