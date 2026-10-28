// @vitest-environment nuxt
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import DigestCard from '~/components/message/aside/DigestCard.vue'
import NotificationPreference from '~/components/message/NotificationPreference.vue'
import type { Notification } from '#shared/utils/api/schemas'

const digestNotification = (over: Partial<Notification> = {}): Notification => ({
  object: 'notification',
  id: '55001',
  notification_type: 'notification_digest',
  actor: {
    object: 'user',
    id: '0',
    name: null,
    avatar: null,
    avatar_frame: null
  },
  actor_count: 1,
  item_count: 1,
  path: '/message/digest/55001',
  excerpt_markdown: '夜间通知摘要：2 个话题有 5 条新回复',
  origin: 'local',
  is_read: false,
  created_at: '2026-10-28T00:00:00Z',
  ...over
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('digest notification card', () => {
  it('links to the digest page and shows an unread badge', async () => {
    const wrapper = await mountSuspended(DigestCard, {
      props: { notification: digestNotification() }
    })
    expect(wrapper.text()).toContain('夜间通知摘要')
    expect(wrapper.text()).toContain('2 个话题有 5 条新回复')
    expect(wrapper.text()).toContain('未读')
    const link = wrapper.find('a')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toContain('/message/digest/55001')
  })

  it('drops the badge once read', async () => {
    const wrapper = await mountSuspended(DigestCard, {
      props: { notification: digestNotification({ is_read: true }) }
    })
    expect(wrapper.text()).not.toContain('未读')
  })
})

describe('notification preference quiet hours', () => {
  it('renders the switch and writes is_quiet_hours_enabled on toggle', async () => {
    const calls: RequestInit[] = []
    const fetchSpy = vi.fn(
      async (input: Request | string, init?: RequestInit) => {
        const url = typeof input === 'string' ? input : input.url
        const method =
          (typeof input !== 'string' ? input.method : undefined) ??
          init?.method ??
          'GET'
        if (url.includes('/notification-preferences') && method === 'PUT') {
          const body =
            init?.body ??
            (typeof input !== 'string' ? await input.clone().text() : '')
          calls.push({ body } as RequestInit)
          return new Response(
            JSON.stringify({
              object: 'notification_preferences',
              muted_types: [],
              is_quiet_hours_enabled: false
            }),
            { headers: { 'content-type': 'application/json' } }
          )
        }
        return new Response(
          JSON.stringify({
            object: 'notification_preferences',
            muted_types: [],
            is_quiet_hours_enabled: true
          }),
          { headers: { 'content-type': 'application/json' } }
        )
      }
    )
    vi.stubGlobal('fetch', fetchSpy)

    const wrapper = await mountSuspended(NotificationPreference)
    await flushPromises()
    expect(wrapper.text()).toContain('夜间免打扰')
    expect(wrapper.text()).toContain('23:00')
    const kunSwitches = wrapper.findAllComponents({ name: 'KunSwitch' })
    const quietSwitchComponent = kunSwitches.at(0)
    if (!quietSwitchComponent) {
      throw new Error('quiet switch component not rendered')
    }
    quietSwitchComponent.vm.$emit('update:modelValue', false)
    await flushPromises()

    expect(calls).toHaveLength(1)
    const body = JSON.parse(calls[0]!.body as string)
    expect(body.is_quiet_hours_enabled).toBe(false)
    expect(Array.isArray(body.muted_types)).toBe(true)
  })
})
