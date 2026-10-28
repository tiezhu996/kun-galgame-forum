-- 220: a topic draft becomes a mutable working copy with pinned revisions.
--
-- Before 220 every save POSTed a new immutable draft, so one long post reached
-- the 30-draft cap with a pile of near-identical rows nobody could tell apart.
-- The editor now PATCHes one draft (autosave, no revision row) and pins a
-- revision only on an explicit save. topic_draft.revision is the optimistic
-- lock: a second window editing the same draft sends the revision it loaded,
-- and a stale write is refused with DRAFT_REVISION_CONFLICT instead of
-- overwriting the other window's content.
--
-- topic_draft_revision holds pinned content. kind 'manual' is an explicit
-- save (and the revision 1 every new draft is born with); kind 'checkpoint'
-- is content the server preserved automatically -- the other window's working
-- copy before a force-overwrite, or the working copy right before a restore.
-- content and cover_images keep /image/<hash> tokens and prize_image_hashes
-- keeps bare hashes, so cron/reference_ping.go discovers them through its
-- text-column scan and the *image_hashes jsonb name scan (see migration 215).
--
-- Existing drafts start at revision 1 and get one 'manual' row backfilled
-- from their own content, so an old draft also has a restorable starting
-- point. Additive: migrate before or with the deploy.
-- Idempotent.
BEGIN;

ALTER TABLE topic_draft
  ADD COLUMN IF NOT EXISTS revision INTEGER NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS topic_draft_revision (
    id                 BIGSERIAL    PRIMARY KEY,
    draft_id           BIGINT       NOT NULL REFERENCES topic_draft (id) ON DELETE CASCADE,
    user_id            INTEGER      NOT NULL,
    kind               VARCHAR(16)  NOT NULL DEFAULT 'manual',
    base_revision      INTEGER      NOT NULL DEFAULT 1,
    title              VARCHAR(233) NOT NULL DEFAULT '',
    content            TEXT         NOT NULL DEFAULT '',
    category           VARCHAR(32)  NOT NULL DEFAULT '',
    sections           TEXT         NOT NULL DEFAULT '',
    cover_images       TEXT         NOT NULL DEFAULT '',
    is_nsfw            BOOLEAN      NOT NULL DEFAULT false,
    polls              jsonb        NOT NULL DEFAULT '[]'::jsonb,
    lotteries          jsonb        NOT NULL DEFAULT '[]'::jsonb,
    prize_image_hashes jsonb        NOT NULL DEFAULT '[]'::jsonb,
    created            TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- A draft's history, newest first; the cascade needs draft_id indexed too.
CREATE INDEX IF NOT EXISTS idx_topic_draft_revision_draft
  ON topic_draft_revision (draft_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_topic_draft_revision_user
  ON topic_draft_revision (user_id);

-- One starting revision per pre-220 draft. Drafts born through the new API
-- write their own revision 1 in the create transaction.
INSERT INTO topic_draft_revision
  (draft_id, user_id, kind, base_revision,
   title, content, category, sections, cover_images, is_nsfw,
   polls, lotteries, prize_image_hashes, created)
SELECT d.id, d.user_id, 'manual', d.revision,
   d.title, d.content, d.category, d.sections, d.cover_images, d.is_nsfw,
   d.polls, d.lotteries, d.prize_image_hashes, d.updated
FROM topic_draft d
WHERE NOT EXISTS (
  SELECT 1 FROM topic_draft_revision r WHERE r.draft_id = d.id
);

COMMIT;
