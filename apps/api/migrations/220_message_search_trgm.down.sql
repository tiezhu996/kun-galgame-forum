-- 220 down: drop the inbox search trgm indexes. Keep pg_trgm; 088 owns it.

BEGIN;

DROP INDEX IF EXISTS idx_message_content_search_trgm;
DROP INDEX IF EXISTS idx_chat_message_content_search_trgm;

COMMIT;
