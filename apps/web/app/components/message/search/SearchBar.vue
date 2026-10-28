<script setup lang="ts">
import type { MessageSearchWithin } from '#shared/utils/api/schemas'

const props = defineProps<{
  q: string
  senderId: string
  within: MessageSearchWithin
}>()

const emit = defineEmits<{
  search: [payload: { q: string; senderId: string; within: MessageSearchWithin }]
  'update:senderId': [value: string]
}>()

const draft = ref(props.q)
const within = ref<MessageSearchWithin>(props.within)
const senderId = ref(props.senderId)

watch(
  () => [props.q, props.within, props.senderId] as const,
  ([q, w, s]) => {
    draft.value = q
    within.value = w
    senderId.value = s
  }
)

const windows: { value: MessageSearchWithin; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: '7d', label: '最近 7 天' },
  { value: '30d', label: '最近 30 天' }
]

const submit = () =>
  emit('search', {
    q: draft.value.trim(),
    senderId: senderId.value,
    within: within.value
  })

const clearAll = () => {
  draft.value = ''
  senderId.value = ''
  within.value = 'all'
  emit('update:senderId', '')
  emit('search', { q: '', senderId: '', within: 'all' })
}
</script>

<template>
  <form class="flex flex-col gap-2" @submit.prevent="submit">
    <div class="flex items-center gap-2">
      <KunInput
        v-model="draft"
        class-name="flex-1"
        placeholder="搜索通知和私聊中的关键词…"
        is-clearable
        autofocus
        @keydown.enter.prevent="submit"
      >
        <template #prefix>
          <KunIcon name="lucide:search" class="text-default-400" />
        </template>
      </KunInput>
      <KunButton type="submit" color="primary">搜索</KunButton>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <div class="border-default-200 flex items-center rounded-lg border p-0.5">
        <button
          v-for="option of windows"
          :key="option.value"
          type="button"
          :class="
            cn(
              'rounded-md px-3 py-1 text-sm transition-colors',
              within === option.value
                ? 'bg-primary text-primary-foreground'
                : 'text-default-500 hover:text-default-800'
            )
          "
          @click="within = option.value; submit()"
        >
          {{ option.label }}
        </button>
      </div>

      <MessageSearchSenderFilter
        :model-value="senderId"
        @update:model-value="(value: string) => {
          senderId = value
          emit('update:senderId', value)
        }"
      />

      <KunButton
        v-if="q || senderId || within !== 'all'"
        size="sm"
        variant="light"
        @click="clearAll"
      >
        <span class="flex items-center gap-1">
          <KunIcon name="lucide:x" />
          清除筛选
        </span>
      </KunButton>
    </div>
  </form>
</template>
