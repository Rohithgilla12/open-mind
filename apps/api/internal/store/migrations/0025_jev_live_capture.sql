-- Phase 2 live capture: persist which tags were auto-applied / suggested so the
-- UI can offer one-tap chips and undo, and record dismissals without losing the
-- full answers payload used for eval / threshold tuning.
ALTER TABLE jev_decisions
    ADD COLUMN applied_tags   text[] NOT NULL DEFAULT '{}',
    ADD COLUMN suggested_tags text[] NOT NULL DEFAULT '{}',
    ADD COLUMN dismissed_tags text[] NOT NULL DEFAULT '{}';
