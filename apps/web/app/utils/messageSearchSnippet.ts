import { markdownToText } from '#shared/utils/markdownToText'

export interface SnippetParts {
  before: string
  hit: string
  after: string
}

const windowRunes = 24
const maxHeadRunes = windowRunes * 2 + 1
const ellipsis = '…'

const toRunes = (s: string) => Array.from(s)

// Locate the phrase case-insensitively, returning rune offsets into the
// ORIGINAL text. The match runs on lowercased strings (so folding can change
// length, e.g. Turkish İ and German ẞ), and the UTF-16 hit span is mapped back
// onto original code-point boundaries by accumulating each original rune's
// folded length. Comparing the two arrays directly would misalign after any
// length-changing fold.
const locateRunBounds = (
  runes: string[],
  phraseRunes: string[]
): [number, number] | null => {
  const lower = runes.map((rune) => rune.toLowerCase())
  const qLower = phraseRunes.map((rune) => rune.toLowerCase()).join('')
  if (!qLower) {
    return null
  }
  const lowerText = lower.join('')
  const unitIdx = lowerText.indexOf(qLower)
  if (unitIdx < 0) {
    return null
  }
  const unitEnd = unitIdx + qLower.length

  let units = 0
  let start = -1
  let end = -1
  for (let i = 0; i < runes.length; i++) {
    if (start < 0 && units >= unitIdx) {
      start = i
    }
    units += lower[i]!.length
    if (end < 0 && units >= unitEnd) {
      end = i + 1
    }
    if (start >= 0 && end >= 0) {
      return [start, end]
    }
  }
  return start >= 0 ? [start, runes.length] : null
}

// Cut a plain-text window around the first phrase hit in the stored markdown.
// The server guarantees the row matched; a missing local hit (e.g. locale
// folding it cannot reproduce) degrades to a head-only window.
export const buildSnippet = (
  markdown: string,
  phrase: string
): SnippetParts => {
  const text = markdownToText(markdown).replace(/\s+/g, ' ').trim()
  const runes = toRunes(text)
  const bounds = locateRunBounds(runes, toRunes(phrase.trim()))

  if (!bounds) {
    const head = runes.slice(0, maxHeadRunes).join('')
    return {
      before: '',
      hit: head,
      after: runes.length > maxHeadRunes ? ellipsis : ''
    }
  }

  const [idx, end] = bounds
  const beforeRunes = runes.slice(0, idx)
  const afterRunes = runes.slice(end)
  const before = beforeRunes.slice(-windowRunes).join('')
  const after = afterRunes.slice(0, windowRunes).join('')

  return {
    before: (beforeRunes.length > windowRunes ? ellipsis : '') + before,
    hit: runes.slice(idx, end).join(''),
    after: after + (afterRunes.length > windowRunes ? ellipsis : '')
  }
}
