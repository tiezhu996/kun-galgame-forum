-- 220: quiz re-practice. galgame_quiz_answer stays one row per (quiz, user);
-- repeat answers overwrite the latest attempt instead of 409 ALREADY_EXISTS.
-- Public stats (galgame_quiz.answer_count / correct_count) and the author
-- notification keep counting each answerer's FIRST attempt, so the first
-- submission and its grade are frozen in first_submitted / first_is_correct.
-- best_* sticky-records whether the user has ever answered correctly: once
-- true it never flips back, which is what removes the quiz from the wrong set.
--
-- Backfill: every pre-220 answerer row holds exactly one attempt, so first,
-- latest and best are all that one row. Author rows stay NULL (default
-- best_is_correct false) and are ignored by every count.
--
-- Idempotent.
BEGIN;

ALTER TABLE galgame_quiz_answer
    ADD COLUMN IF NOT EXISTS first_submitted JSONB,
    ADD COLUMN IF NOT EXISTS first_is_correct BOOLEAN,
    ADD COLUMN IF NOT EXISTS best_submitted JSONB,
    ADD COLUMN IF NOT EXISTS best_is_correct BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS best_at TIMESTAMPTZ;

UPDATE galgame_quiz_answer
SET first_submitted = submitted,
    first_is_correct = is_correct,
    best_submitted = submitted,
    best_is_correct = COALESCE(is_correct, false),
    best_at = created
WHERE role = 'answerer'
  AND first_submitted IS NULL;

COMMIT;
