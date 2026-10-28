-- 220: admin-run merges of duplicate website directory rows.
--
-- The same site is sometimes listed twice under different hosts. Likes,
-- favorites and tag links are per website row, and comments live on a
-- community wall anchored at website:<id>, so the two listings never share
-- anything and the only repair was deleting one row and re-adding the other
-- by hand. An admin merge folds one row ("old") onto the survivor ("new") in
-- one transaction: likes and favorites move with ON CONFLICT DO NOTHING (a
-- user who engaged both counts once), tags move unless the survivor already
-- carries a tag of the same single-select group, counters and views are
-- recomputed, and the old row is deleted.
--
-- This table is both the audit ledger and the redirect map:
--   old_website_id has no foreign key because the fold deletes that row.
--   old_host is what /website/<host> matched before; the host read answers
--   ENTITY_MERGED naming the survivor, and the page 301-redirects.
--   When the survivor itself is later folded away, the fold re-points every
--   row's new_website_id at the newest survivor instead of leaving a chain.
--
-- old_row is the deleted website row as jsonb. The dropped_* arrays record
-- which user ids / tag ids the fold could not move, so the decision is not
-- silent; likes and favorites carry no written content of their own.
--
-- Existing rows: none. The table starts empty and nothing reads it until an
-- admin runs a merge. Additive: migrate before or with the deploy.

CREATE TABLE IF NOT EXISTS galgame_website_merge (
    id                     bigserial   PRIMARY KEY,
    old_website_id         integer     NOT NULL,
    new_website_id         integer     NOT NULL,
    old_host               text        NOT NULL,
    old_row                jsonb       NOT NULL,
    moved_like             integer     NOT NULL DEFAULT 0,
    moved_favorite         integer     NOT NULL DEFAULT 0,
    dropped_like           integer     NOT NULL DEFAULT 0,
    dropped_favorite       integer     NOT NULL DEFAULT 0,
    moved_tag              integer     NOT NULL DEFAULT 0,
    dropped_tag            integer     NOT NULL DEFAULT 0,
    added_urls             jsonb       NOT NULL DEFAULT '[]',
    dropped_like_users     jsonb,
    dropped_favorite_users jsonb,
    dropped_tags           jsonb,
    comment_count_old      integer,
    view_old               integer,
    created_by             integer,
    created                timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_galgame_website_merge_old_id
    ON galgame_website_merge (old_website_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_galgame_website_merge_old_host
    ON galgame_website_merge (old_host);
CREATE INDEX IF NOT EXISTS idx_galgame_website_merge_new
    ON galgame_website_merge (new_website_id);

SELECT user_purge_archive_attach();
