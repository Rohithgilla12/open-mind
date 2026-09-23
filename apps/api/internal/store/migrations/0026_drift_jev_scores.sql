-- Phase 4: precomputed Drift blend scores from the daily Jev scoring job.
-- NULL means unscored (keep recency-heuristic order). GET /drift reorders by
-- drift_score when scores are fresh; the request path never calls Jev.
ALTER TABLE items ADD COLUMN drift_score real;
ALTER TABLE items ADD COLUMN drift_scored_at timestamptz;
