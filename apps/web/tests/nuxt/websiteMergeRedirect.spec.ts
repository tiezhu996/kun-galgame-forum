// @vitest-environment nuxt
import { describe, expect, it } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import { computed, ref } from 'vue'
import WebsiteDetailPage from '~/pages/website/[domain].vue'

const merged = {
  code: 'ENTITY_MERGED',
  status: 404,
  object: 'website',
  current_id: '31',
  current_host: 'keep.example'
}

const navigateCalls: Array<{ to: unknown; opts?: unknown }> = []

mockNuxtImport(
  'useKunApi',
  () =>
    async (
      _key: unknown,
      fetcher: (api: unknown) => Promise<unknown>
    ) => {
      const api = { GET: async () => ({ error: merged, data: undefined }) }
      await fetcher(api)
      return {
        data: computed(() => undefined),
        problem: computed(() => ({ status: 404, code: 'ENTITY_MERGED' })),
        status: ref('error'),
        refresh: () => {}
      }
    }
)

mockNuxtImport(
  'navigateTo',
  () =>
    (to: unknown, opts?: unknown) => {
      navigateCalls.push({ to, opts })
      return Promise.resolve()
    }
)

describe('website detail of a merged host', () => {
  it('301-redirects to the survivor host', async () => {
    await mountSuspended(WebsiteDetailPage, {
      route: '/website/gone.example'
    })
    expect(navigateCalls.length).toBeGreaterThan(0)
    expect(navigateCalls[0]).toEqual({
      to: '/website/keep.example',
      opts: { redirectCode: 301, replace: true }
    })
  })
})
