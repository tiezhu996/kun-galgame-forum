<script setup lang="ts">
import type { NotificationDigestDetail } from '#shared/utils/api/schemas'
import { problemMessage } from '#shared/utils/api/message'
import { settle } from '#shared/utils/api/problem'

definePageMeta({
  middleware: 'auth'
})

useKunDisableSeo('夜间通知摘要')

useHead({ title: `夜间通知摘要 - ${kungal.titleShort}` })

const route = useRoute()
const client = useApiClient()
const noticeEpoch = useState('message-notice-epoch', () => 0)
const router = useRouter()

const digestId = computed(() => String(route.params.id ?? ''))

const {
  data: digest,
  status,
  problem,
  refresh
} = useKunApi<NotificationDigestDetail>(
  () => `me-notification-digest-${digestId.value}`,
  (api, { signal }) =>
    api.GET('/me/notifications/digests/{digest_id}', {
      params: { path: { digest_id: digestId.value } },
      signal
    })
)

const isMarking = ref(false)

const windowLabel = computed(() => {
  const d = digest.value
  if (!d) {
    return ''
  }
  const start = new Date(d.window_start_at)
  const end = new Date(d.window_end_at)
  const fmt = (t: Date) =>
    `${t.getMonth() + 1} 月 ${t.getDate()} 日 ${String(t.getHours()).padStart(2, '0')}:${String(t.getMinutes()).padStart(2, '0')}`
  return `${fmt(start)} – ${fmt(end)}`
})

const markAllRead = async () => {
  if (isMarking.value || digest.value?.is_read) {
    return
  }
  isMarking.value = true
  const result = await settle(
    client.PUT('/me/notifications/digests/{digest_id}/read', {
      params: { path: { digest_id: digestId.value } }
    })
  )
  isMarking.value = false
  if (!result.ok) {
    reportProblem(result.problem)
    return
  }
  noticeEpoch.value += 1
  await router.push('/message/notice')
}
</script>

<template>
  <div class="flex w-full flex-col space-y-3">
    <header class="flex items-center gap-2">
      <KunButton
        size="lg"
        :is-icon-only="true"
        variant="light"
        href="/message/notice"
      >
        <KunIcon name="lucide:chevron-left" />
      </KunButton>
      <h2 class="text-lg">夜间通知摘要</h2>
    </header>

    <KunDivider />

    <MessageAsideNoticeSkeleton v-if="status === 'pending'" />

    <template v-else-if="problem">
      <KunNull :description="problemMessage(problem)" />
      <div class="flex justify-center">
        <KunButton variant="flat" size="sm" @click="() => refresh()">
          重试
        </KunButton>
      </div>
    </template>

    <template v-else-if="digest">
      <div class="text-default-500 text-sm">{{ windowLabel }}</div>

      <section
        v-if="digest.topic_groups.length"
        class="space-y-1"
        aria-label="话题新回复"
      >
        <h3 class="text-default-700 px-1 text-sm font-bold">话题新回复</h3>
        <KunLink
          v-for="group in digest.topic_groups"
          :key="group.topic_id"
          color="default"
          underline="none"
          class-name="hover:bg-primary/20 flex cursor-pointer items-center justify-between gap-3 rounded-lg p-3 transition-colors"
          :to="group.path"
        >
          <span class="line-clamp-2 break-all text-sm font-medium">
            {{ group.topic_title || '该话题已不可用' }}
          </span>
          <span class="text-default-500 flex shrink-0 items-center gap-2 text-sm">
            <KunChip color="primary">{{ group.count }} 条</KunChip>
            <KunIcon name="lucide:chevron-right" />
          </span>
        </KunLink>
      </section>

      <section
        v-if="digest.other_items.length"
        class="space-y-1"
        aria-label="其他互动"
      >
        <h3 class="text-default-700 px-1 pt-2 text-sm font-bold">其他互动</h3>
        <MessageAsideNotice
          v-for="notification in digest.other_items"
          :key="notification.id"
          :notification="notification"
        />
      </section>

      <KunDivider />

      <div class="flex justify-end">
        <KunButton
          color="primary"
          :loading="isMarking"
          :disabled="digest.is_read"
          @click="markAllRead"
        >
          {{ digest.is_read ? '已全部标为已读' : '全部标为已读' }}
        </KunButton>
      </div>
    </template>

    <KunNull v-else />
  </div>
</template>
