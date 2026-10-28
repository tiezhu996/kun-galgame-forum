import type { TopicAccessRole, TopicAccessScope } from '~/constants/topic'
import type { PollFormData } from '~/components/topic/poll/types'
import type { LotteryFormData } from '~/components/topic/lottery/types'

export interface EditStorePersist {
  mode: 'preview' | 'code'

  title: string
  content: string
  section: string[]
  isNSFW: boolean
  coverImages: string[]
  accessScope: TopicAccessScope
  accessRoles: TopicAccessRole[]
  accessUserIds: number[]
  polls: PollFormData[]
  lotteries: LotteryFormData[]

  // The server draft this editor is currently saving into: '' means no draft
  // has been created yet. draftRevision is the optimistic-lock token last
  // returned by the server. Persisted so reopening the page continues the
  // same draft instead of spawning a new one.
  draftId: string
  draftRevision: number
}

export interface EditStoreTemp {
  id: number
  title: string
  content: string
  section: string[]
  isNSFW: boolean
  coverImages: string[]
  accessScope: TopicAccessScope
  accessRoles: TopicAccessRole[]
  accessUserIds: number[]
  accessUsers: KunUser[]

  isTopicRewriting: boolean
}
