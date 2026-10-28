<script setup lang="ts">
import {
  draftDiffText,
  lineDiff,
  type DraftDiffSource
} from '~/utils/topicDraftDiff'

const props = defineProps<{
  oldSource: DraftDiffSource
  newSource: DraftDiffSource
}>()

const lines = computed(() =>
  lineDiff(draftDiffText(props.oldSource), draftDiffText(props.newSource))
)
</script>

<template>
  <div
    class="border-default-200 overflow-hidden rounded-lg border text-xs leading-5"
  >
    <div
      class="bg-default-100 border-default-200 text-default-500 grid grid-cols-2 border-b text-[11px]"
    >
      <div class="px-3 py-1.5">旧内容</div>
      <div class="border-default-200 border-l px-3 py-1.5">新内容</div>
    </div>
    <div class="max-h-[50vh] overflow-auto py-1 font-mono">
      <div
        v-for="(line, i) in lines"
        :key="i"
        :class="cn('grid grid-cols-2', line.type === 'equal' && 'block')"
      >
        <template v-if="line.type === 'equal'">
          <div class="text-default-500 px-3 whitespace-pre-wrap">
            {{ line.oldText || ' ' }}
          </div>
        </template>
        <template v-else-if="line.type === 'add'">
          <div />
          <div
            class="bg-success-500/10 text-success-700 dark:text-success-400 border-success-500/30 border-l-4 px-3 whitespace-pre-wrap"
          >
            {{ line.newText || ' ' }}
          </div>
        </template>
        <template v-else-if="line.type === 'del'">
          <div
            class="bg-danger-500/10 text-danger-600 dark:text-danger-400 border-danger-500/30 border-l-4 px-3 whitespace-pre-wrap"
          >
            {{ line.oldText || ' ' }}
          </div>
          <div />
        </template>
        <template v-else>
          <div
            class="bg-danger-500/10 text-danger-600 dark:text-danger-400 border-danger-500/30 border-l-4 px-3 whitespace-pre-wrap"
          >
            {{ line.oldText ?? ' ' }}
          </div>
          <div
            class="bg-success-500/10 text-success-700 dark:text-success-400 border-success-500/30 border-l-4 px-3 whitespace-pre-wrap"
          >
            {{ line.newText ?? ' ' }}
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
