DROP TABLE IF EXISTS topic_draft_revision;

ALTER TABLE topic_draft
  DROP COLUMN IF EXISTS revision;
