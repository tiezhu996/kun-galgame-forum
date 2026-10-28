import type {
  AdminFriendLinkState,
  FriendLinkCategory,
  FriendLinkState
} from '#shared/utils/api/schemas'

export const FRIEND_LINK_CATEGORIES: {
  key: FriendLinkCategory
  label: string
}[] = [
  { key: 'official', label: '官方网站' },
  { key: 'galgame', label: 'Galgame 网站' },
  { key: 'others', label: '其它网站' }
]

export const FRIEND_LINK_CATEGORY_OPTIONS = FRIEND_LINK_CATEGORIES.map((c) => ({
  value: c.key,
  label: c.label
}))

// Staff may only pick normal/down; abnormal is checker-owned and reaches the
// form only as a banner explaining the rescue action.
export const FRIEND_LINK_STATE_OPTIONS: {
  value: FriendLinkState
  label: string
}[] = [
  { value: 'normal', label: '正常' },
  { value: 'down', label: '已下线' }
]

// Public chips: the public list never contains abnormal rows.
export const FRIEND_LINK_STATE_CHIP: Record<
  FriendLinkState,
  { label: string; color: 'danger' } | null
> = {
  normal: null,
  down: { label: '已下线', color: 'danger' }
}

// Admin chips cover the checker-owned abnormal state too.
export const FRIEND_LINK_ADMIN_STATE_CHIP: Record<
  AdminFriendLinkState,
  { label: string; color: 'danger' | 'warning' } | null
> = {
  normal: null,
  down: { label: '已下线', color: 'danger' },
  abnormal: { label: '异常', color: 'warning' }
}
