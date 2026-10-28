<script setup lang="ts">
import type {
  MessageSearchResult,
  Notification
} from '#shared/utils/api/schemas'
import { markdownToText } from '#shared/utils/markdownToText'
import { deletedUserName, toKunUser } from '~/utils/userRef'
import { getMessageI18n } from '../utils/getMessageI18n'
import { buildSnippet } from '~/utils/messageSearchSnippet'

const props = defineProps<{
  result: MessageSearchResult
  phrase: string
}>()

const senderName = computed(() => props.result.sender.name ?? deletedUserName)

const senderUser = computed<KunUser>(() => ({
  ...toKunUser(props.result.sender),
  name:
    props.result.hit === 'notification' &&
    props.result.notification_type === 'system_notice'
      ? '系统通知'
      : senderName.value
}))

const isNotification = computed(() => props.result.hit === 'notification')

const href = computed(() => {
  if (isNotification.value) {
    return props.result.path ?? '/'
  }
  return props.result.peer ? `/message/user/${props.result.peer.id}` : '/'
})

const pseudoNotification = computed<Notification | null>(() => {
  if (!isNotification.value || !props.result.notification_type) {
    return null
  }
  return {
    object: 'notification',
    id: props.result.id,
    notification_type: props.result.notification_type,
    actor: props.result.sender,
    actor_count: 1,
    item_count: 1,
    path: props.result.path ?? '/',
    excerpt_markdown: props.result.snippet_markdown,
    origin: 'local',
    is_read: true,
    created_at: props.result.created_at
  }
})

const actionText = computed(() => {
  const n = pseudoNotification.value
  return n ? getMessageI18n(n) : ''
})

const peerName = computed(
  () => props.result.peer?.name ?? deletedUserName
)

const isRecalled = computed(
  () => !isNotification.value && props.result.state === 'recalled'
)

const snippet = computed(() =>
  buildSnippet(props.result.snippet_markdown, props.phrase)
)

const snippetHasText = computed(() =>
  markdownToText(props.result.snippet_markdown).trim().length > 0
)
</script>

<template>
  <KunLink
    color="default"
    underline="none"
    class-name="border-default-200/60 hover:bg-primary/10 flex w-full flex-col gap-2 rounded-lg border p-3 transition-colors"
    :to="href"
  >
    <div class="flex items-center gap-2">
      <KunAvatar :user="senderUser" size="md" :is-navigation="false" />
      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
          <span class="font-bold">{{ senderUser.name }}</span>
          <template v-if="isNotification">
            <span class="text-default-600">{{ actionText }}</span>
            <KunChip size="sm" color="primary">通知</KunChip>
            <span
              v-if="result.topic_title"
              class="text-default-500 truncate text-xs"
            >
              话题：{{ result.topic_title }}
            </span>
          </template>
          <template v-else>
            <KunChip size="sm" color="secondary">私信</KunChip>
            <span class="text-default-500 truncate text-xs">
              与 {{ peerName }} 的对话
            </span>
          </template>
        </div>
      </div>
      <span class="text-default-500 shrink-0 text-xs">
        <KunTime :time="result.created_at" type="datetime" show-year />
      </span>
    </div>

    <pre
      class="text-default-700 break-word whitespace-pre-line text-sm leading-6"
    ><template v-if="isRecalled"><span class="text-default-500">{{ senderName }}撤回了一条消息</span></template><template v-else-if="snippetHasText"><span class="text-default-400">{{ snippet.before }}</span><mark class="bg-primary/20 rounded px-0.5 text-inherit">{{ snippet.hit }}</mark><span class="text-default-400">{{ snippet.after }}</span></template><template v-else><span class="text-default-400">点击查看详情</span></template></pre>
  </KunLink>
</template>
