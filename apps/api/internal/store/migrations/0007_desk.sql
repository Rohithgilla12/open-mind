ALTER TABLE items ADD COLUMN pinned_at timestamptz;

-- Partial index: Desk queries only ever read pinned rows, ordered newest-pinned
-- first, so index just those rows on (user_id, pinned_at DESC).
CREATE INDEX items_user_pinned_idx ON items (user_id, pinned_at DESC) WHERE pinned_at IS NOT NULL;
