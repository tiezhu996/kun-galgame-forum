import { describe, expect, it } from 'vitest'
import { mergedInto, mergedWebsiteHost } from './merged'

describe('mergedInto', () => {
  it('reads the survivor id off an ENTITY_MERGED body', () => {
    expect(
      mergedInto({ code: 'ENTITY_MERGED', current_id: '42' })
    ).toBe(42)
  })

  it('ignores other problems and malformed ids', () => {
    expect(mergedInto({ code: 'NOT_FOUND' })).toBeNull()
    expect(
      mergedInto({ code: 'ENTITY_MERGED', current_id: 'nope' })
    ).toBeNull()
    expect(mergedInto(undefined)).toBeNull()
  })
})

describe('mergedWebsiteHost', () => {
  it('reads the survivor host off a website merge body', () => {
    expect(
      mergedWebsiteHost({
        code: 'ENTITY_MERGED',
        object: 'website',
        current_id: '42',
        current_host: 'keep.example'
      })
    ).toBe('keep.example')
  })

  it('stays silent for non-website merges and other problems', () => {
    expect(
      mergedWebsiteHost({ code: 'ENTITY_MERGED', object: 'work', current_id: '9' })
    ).toBeNull()
    expect(
      mergedWebsiteHost({
        code: 'ENTITY_MERGED',
        object: 'website',
        current_id: '9'
      })
    ).toBeNull()
    expect(mergedWebsiteHost({ code: 'NOT_FOUND' })).toBeNull()
  })
})
