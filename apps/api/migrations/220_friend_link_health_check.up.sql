-- 220: automatic friend-link health checks.
--
-- status gains the value 'abnormal'. It is checker-owned: after
-- health_fail_count reaches 3 consecutive failed probes the cron sets it and
-- the public list hides the row; after health_success_count reaches 2
-- consecutive successes the cron restores 'normal'. Staff may only write
-- 'normal'/'down' through the API, so a patch to 'normal' is the manual
-- rescue path. 'down' rows are still probed and get fresh check metadata,
-- but the checker must never change their status or counters.
--
-- last_fail_reason is the latest failure explanation only. A failure with an
-- empty reason keeps the previous value (a bare timeout used to blank it),
-- and the column is cleared on recovery. It is never the link's description.
BEGIN;

ALTER TABLE friend_link
  ADD COLUMN IF NOT EXISTS health_fail_count    integer     NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS health_success_count integer     NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_checked_at      timestamptz,
  ADD COLUMN IF NOT EXISTS last_fail_reason     text        NOT NULL DEFAULT '';

COMMIT;
