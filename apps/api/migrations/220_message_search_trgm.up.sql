-- 220: pg_trgm GIN indexes for GET /me/message-search.
--
-- WHY: message-search matches a case-insensitive substring on message.content
-- (notification excerpt, varchar(233)) and chat_message.content (direct
-- message markdown, varchar(1000)) with ILIKE '%phrase%'. Without a trgm index
-- the notification branch scans every row of the receiver's index range and
-- the direct-message branch scans every room message of the caller's rooms.
-- gin_trgm_ops supports leading-wildcard ILIKE, as 088 already does for topic
-- search: a selective phrase gives a trgm bitmap, and the notification branch
-- can combine it with the receiver key from migration 130. Short CJK phrases
-- (fewer than three trigrams) or near-everything phrases cannot use the GIN
-- index; those plans fall back to the migration-130 receiver key
-- (notification) or a per-user scan, the same trade-off 088 accepted.
--
-- Additive only: deploy order does not matter and no row is touched. The
-- pg_trgm extension is owned by 088 and is left in place on rollback.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_message_content_search_trgm
  ON message USING gin (content gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_chat_message_content_search_trgm
  ON chat_message USING gin (content gin_trgm_ops);

COMMIT;
