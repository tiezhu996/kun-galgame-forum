<script setup lang="ts">
import type { Notification } from '#shared/utils/api/schemas'
import { markdownToText } from '#shared/utils/markdownToText'

const props = defineProps<{
  notification: Notification
}>()

const preview = computed(() =>
  markdownToText(props.notification.excerpt_markdown).trim()
)
</script>

<template>
  <KunLink
    color="default"
    underline="none"
    class-name="bg-primary/10 hover:bg-primary/20 flex cursor-pointer items-center gap-3 rounded-lg p-3 transition-colors"
    :to="`/message/digest/${notification.id}`"
  >
    <div class="bg-primary/20 rounded-full p-2">
      <KunIcon name="lucide:moon-star" class="text-primary text-xl" />
    </div>

    <div class="min-w-0 flex-1 space-y-1">
      <div class="flex items-center justify-between gap-2">
        <span class="font-bold">夜间通知摘要</span>
        <span class="text-default-500 shrink-0 text-sm">
          <KunTime :time="notification.created_at" type="datetime" show-year />
        </span>
      </div>
      <div class="flex items-center justify-between gap-2">
        <span class="line-clamp-2 break-all text-sm">
          {{ preview || '点击查看昨晚的通知摘要' }}
        </span>
        <KunChip
          v-if="!notification.is_read"
          class-name="shrink-0"
          color="primary"
        >
          未读
        </KunChip>
      </div>
    </div>

    <KunIcon name="lucide:chevron-right" class="text-default-400" />
  </KunLink>
</template>
