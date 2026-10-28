import { describe, expect, it } from 'vitest'
import { buildSnippet } from './messageSearchSnippet'

describe('buildSnippet', () => {
  it('windows around the first case-insensitive hit', () => {
    const snippet = buildSnippet(
      '前面是一些无关的上下文文字，补丁MARKER在这里出现，后面还有更多对话内容',
      'marker'
    )
    expect(snippet.hit).toBe('MARKER')
    expect(snippet.before.endsWith('补丁')).toBe(true)
    expect(snippet.after.startsWith('在这里')).toBe(true)
  })

  it('marks a mixed-case English phrase', () => {
    const snippet = buildSnippet('the Patch arrived yesterday', 'patch')
    expect(snippet.hit).toBe('Patch')
    expect(snippet.before).toBe('the ')
    expect(snippet.after.startsWith(' arrived')).toBe(true)
  })

  it('adds ellipses only on the clipped sides', () => {
    const longBefore = 'a'.repeat(40)
    const longAfter = 'b'.repeat(40)
    const snippet = buildSnippet(`${longBefore}X${longAfter}`, 'x')
    expect(snippet.before.startsWith('…')).toBe(true)
    expect(snippet.after.endsWith('…')).toBe(true)
  })

  it('strips image and link markdown to plain text', () => {
    const snippet = buildSnippet(
      '看这个 ![图](https://example.test/a.webp) [补丁](https://example.test) 就行',
      '补丁'
    )
    expect(snippet.before).not.toContain('![')
    expect(snippet.hit).toBe('补丁')
  })

  it('collapses newlines into one line', () => {
    const snippet = buildSnippet('第一行\n第二行 补丁关键词\n第四行', '补丁关键词')
    expect(snippet.before.endsWith('第二行 ')).toBe(true)
    expect(snippet.hit).toBe('补丁关键词')
  })

  it('keeps correct slice points when emoji precede the hit', () => {
    const snippet = buildSnippet('😀🎉 补丁来了快收', '补丁')
    expect(snippet.hit).toBe('补丁')
    expect(snippet.before).toBe('😀🎉 ')
    expect(snippet.after.startsWith('来了快收')).toBe(true)
  })

  it('survives length-changing case folding before the hit', () => {
    const snippet = buildSnippet('İİİ补丁来了', '补丁')
    expect(snippet.hit).toBe('补丁')
    expect(snippet.before).toBe('İİİ')
    expect(snippet.after.startsWith('来了')).toBe(true)
  })

  it('falls back to a head window when the phrase cannot be located', () => {
    const snippet = buildSnippet('abcdef', 'XYZ')
    expect(snippet.before).toBe('')
    expect(snippet.hit).toBe('abcdef')
    expect(snippet.after).toBe('')
  })
})
