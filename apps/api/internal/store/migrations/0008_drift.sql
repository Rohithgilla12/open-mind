-- Drift resurfacing: track when an item was last surfaced in Drift. NULL means
-- never surfaced. Additive, nullable, no default — leaves search_tsv untouched.
ALTER TABLE items ADD COLUMN last_drifted_at timestamptz;
