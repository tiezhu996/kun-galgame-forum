import { describe, expect, it } from 'vitest'
import { draftDiffText, hasDiff, lineDiff } from './topicDraftDiff'

describe('lineDiff', () => {
  it('marks equal, added and removed lines', () => {
    const lines = lineDiff('same\ngone\n', 'same\njoined\n')
    expect(lines[0]).toEqual({
      type: 'equal',
      oldText: 'same',
      newText: 'same'
    })
    expect(lines[1]).toEqual({
      type: 'replace',
      oldText: 'gone',
      newText: 'joined'
    })
    expect(lines[2]).toEqual({ type: 'equal', oldText: '', newText: '' })
  })

  it('treats identical text as all-equal', () => {
    const lines = lineDiff('a\nb', 'a\nb')
    expect(lines.every((l) => l.type === 'equal')).toBe(true)
  })
})

describe('draftDiffText', () => {
  it('includes metadata so section changes are visible', () => {
    const a = draftDiffText({
      title: 't',
      content: 'body',
      sections: ['news'],
      is_nsfw: false
    })
    const b = draftDiffText({
      title: 't',
      content: 'body',
      sections: ['review'],
      is_nsfw: false
    })
    expect(a).toContain('分区：news')
    expect(
      hasDiff(
        { title: 't', content: 'body', sections: ['news'], is_nsfw: false },
        {
          title: 't',
          content: 'body',
          sections: ['news'],
          is_nsfw: false
        }
      )
    ).toBe(false)
    expect(
      hasDiff(
        { title: 't', content: 'body', sections: ['news'], is_nsfw: false },
        { title: 't', content: 'body', sections: [], is_nsfw: false }
      )
    ).toBe(true)
    expect(a).not.toBe(b)
  })
})
