ALTER TABLE friend_link DROP COLUMN IF EXISTS health_fail_count;
ALTER TABLE friend_link DROP COLUMN IF EXISTS health_success_count;
ALTER TABLE friend_link DROP COLUMN IF EXISTS last_checked_at;
ALTER TABLE friend_link DROP COLUMN IF EXISTS last_fail_reason;
