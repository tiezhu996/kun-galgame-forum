-- Drops the quiet-hours digest: trigger, digest tables, and the preference
-- column. Synthetic type='digest' message rows are left in place and simply
-- render as ordinary notifications again; they can be deleted afterwards if
-- needed.

BEGIN;

DROP TRIGGER IF EXISTS trg_message_digest_detach ON message;
DROP FUNCTION IF EXISTS notification_digest_detach_overflow();

DROP TABLE IF EXISTS notification_digest_item;
DROP TABLE IF EXISTS notification_digest;

ALTER TABLE kungal_user_state
  DROP COLUMN IF EXISTS quiet_hours_enabled;

COMMIT;
