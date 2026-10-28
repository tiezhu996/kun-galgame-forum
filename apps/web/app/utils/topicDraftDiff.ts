export type DraftDiffOpType = 'equal' | 'add' | 'del'

export interface DraftDiffOp {
  type: DraftDiffOpType
  text: string
}

// Above this many LCS cells the quadratic table costs more than a diff is
// worth: fall back to "whole block replaced" instead of freezing the tab on
// a pathological 100k-character paste.
const MAX_LCS_CELLS = 4_000_000

const at = (lines: string[], index: number): string => lines[index] ?? ''

const lineOps = (a: string[], b: string[]): DraftDiffOp[] => {
  if (a.length * b.length > MAX_LCS_CELLS) {
    return [
      ...a.map((text) => ({ type: 'del' as const, text })),
      ...b.map((text) => ({ type: 'add' as const, text }))
    ]
  }

  const cell = (rows: number[][], x: number, y: number): number =>
    rows[x]?.[y] ?? 0

  const dp: number[][] = Array.from({ length: a.length + 1 }, () =>
    new Array<number>(b.length + 1).fill(0)
  )
  for (let i = a.length - 1; i >= 0; i--) {
    const row = new Array<number>(b.length + 1).fill(0)
    for (let j = b.length - 1; j >= 0; j--) {
      row[j] =
        at(a, i) === at(b, j)
          ? cell(dp, i + 1, j + 1) + 1
          : Math.max(cell(dp, i + 1, j), row[j + 1] ?? 0)
    }
    dp[i] = row
  }

  const ops: DraftDiffOp[] = []
  let i = 0
  let j = 0
  while (i < a.length && j < b.length) {
    if (at(a, i) === at(b, j)) {
      ops.push({ type: 'equal', text: at(a, i) })
      i++
      j++
    } else if (cell(dp, i + 1, j) >= cell(dp, i, j + 1)) {
      ops.push({ type: 'del', text: at(a, i) })
      i++
    } else {
      ops.push({ type: 'add', text: at(b, j) })
      j++
    }
  }
  while (i < a.length) {
    ops.push({ type: 'del', text: at(a, i) })
    i++
  }
  while (j < b.length) {
    ops.push({ type: 'add', text: at(b, j) })
    j++
  }
  return ops
}

// Merge a run of removed lines immediately followed by added lines into
// replace pairs so the view can show them side by side.
export interface DraftDiffLine {
  type: DraftDiffOpType | 'replace'
  oldText?: string
  newText?: string
}

export const lineDiff = (oldText: string, newText: string): DraftDiffLine[] => {
  const ops = lineOps(oldText.split('\n'), newText.split('\n'))
  const lines: DraftDiffLine[] = []
  for (let k = 0; k < ops.length; k++) {
    const op = ops[k]
    if (!op) {
      continue
    }
    if (op.type === 'del') {
      const removed: string[] = []
      while (k < ops.length && ops[k]?.type === 'del') {
        removed.push(ops[k]?.text ?? '')
        k++
      }
      const added: string[] = []
      while (k < ops.length && ops[k]?.type === 'add') {
        added.push(ops[k]?.text ?? '')
        k++
      }
      k--
      const pairs = Math.max(removed.length, added.length)
      for (let p = 0; p < pairs; p++) {
        lines.push({
          type: 'replace',
          oldText: p < removed.length ? removed[p] : undefined,
          newText: p < added.length ? added[p] : undefined
        })
      }
    } else if (op.type === 'add') {
      lines.push({ type: 'add', newText: op.text })
    } else {
      lines.push({ type: 'equal', oldText: op.text, newText: op.text })
    }
  }
  return lines
}

// The fields a draft comparison needs. Revisions and live drafts share this
// shape; the editor buffer can be mapped onto it too.
export interface DraftDiffSource {
  title: string
  content: string
  sections: string[]
  is_nsfw: boolean
}

// Compose one comparable text: metadata first so a section or NSFW change
// shows up in the same diff as the body.
export const draftDiffText = (d: DraftDiffSource): string =>
  [
    `标题：${d.title}`,
    `分区：${d.sections.join(', ')}`,
    `NSFW：${d.is_nsfw ? '是' : '否'}`,
    '',
    d.content
  ].join('\n')

export const hasDiff = (a: DraftDiffSource, b: DraftDiffSource): boolean =>
  draftDiffText(a) !== draftDiffText(b)
