-- 220: nightly quiet hours and the morning notification digest.
--
-- Quiet hours are one fixed window, 23:00-08:00 Asia/Shanghai, on by default.
-- During the window the read side hides a user's unread, non-muted message
-- rows created inside it (predicate only; writers never change), and the 08:00
-- job groups those rows into one synthetic type='digest' notification per
-- night. notification_digest is the batch metadata; notification_digest_item
-- links member rows. A member stays hidden while its digest is unread.
--
-- Muted types never enter a digest: the builder excludes them and the read
-- side re-checks the current muted set.
--
-- The trigger is load-bearing for community folds. UpsertCommunityMirror
-- moves a fold row's created forward when new replies arrive; a row digested
-- at 08:00 that folds at 10:00 would otherwise stay hidden forever and the
-- new replies would never light the dot. When created moves past the digest
-- window the row is detached and surfaces as a normal unread notification;
-- the digest's snapshot counts are not rolled back.
--
-- Existing users default quiet_hours_enabled to true, so the first 08:00 run
-- after deploy digests the previous night for everyone. Additive: migrate
-- before deploy.

BEGIN;

ALTER TABLE kungal_user_state
  ADD COLUMN IF NOT EXISTS quiet_hours_enabled boolean NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS notification_digest (
    id           bigserial PRIMARY KEY,
    receiver_id  integer NOT NULL,
    message_id   integer NOT NULL REFERENCES message (id) ON DELETE CASCADE,
    window_start timestamptz NOT NULL,
    window_end   timestamptz NOT NULL,
    status       text NOT NULL DEFAULT 'unread',
    total_count  integer NOT NULL DEFAULT 0,
    read_at      timestamptz,
    created      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (receiver_id, window_start),
    UNIQUE (message_id)
);

CREATE INDEX IF NOT EXISTS idx_notification_digest_receiver
  ON notification_digest (receiver_id, created DESC);

CREATE TABLE IF NOT EXISTS notification_digest_item (
    digest_id  bigint NOT NULL REFERENCES notification_digest (id) ON DELETE CASCADE,
    message_id integer NOT NULL REFERENCES message (id) ON DELETE CASCADE,
    PRIMARY KEY (digest_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_notification_digest_item_message
  ON notification_digest_item (message_id);

CREATE OR REPLACE FUNCTION notification_digest_detach_overflow() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.created > OLD.created THEN
        DELETE FROM notification_digest_item di
        USING notification_digest d
        WHERE di.message_id = NEW.id
          AND di.digest_id = d.id
          AND NEW.created >= d.window_end;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER trg_message_digest_detach
    AFTER UPDATE OF created ON message
    FOR EACH ROW
    EXECUTE FUNCTION notification_digest_detach_overflow();

COMMIT;
