-- 220 down: drop the re-practice columns. latest submitted/is_correct remain
-- as the user's visible attempt; first/best history is discarded.
BEGIN;

ALTER TABLE galgame_quiz_answer
    DROP COLUMN IF EXISTS first_submitted,
    DROP COLUMN IF EXISTS first_is_correct,
    DROP COLUMN IF EXISTS best_submitted,
    DROP COLUMN IF EXISTS best_is_correct,
    DROP COLUMN IF EXISTS best_at;

COMMIT;
