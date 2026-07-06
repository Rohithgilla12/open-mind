ALTER TABLE items ADD COLUMN user_tags text[] NOT NULL DEFAULT '{}';

-- Rebuild the STORED generated search_tsv column to also index user_tags.
-- The column must be dropped and re-added (a generated expression cannot be
-- altered in place); the gin index depends on it, so drop and recreate it too.
DROP INDEX items_search_tsv_idx;
ALTER TABLE items DROP COLUMN search_tsv;
ALTER TABLE items ADD COLUMN search_tsv tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('english'::regconfig, title), 'A') ||
    setweight(to_tsvector('english'::regconfig, summary), 'B') ||
    setweight(array_to_tsvector(tags), 'B') ||
    setweight(to_tsvector('english'::regconfig, left(body, 100000)), 'C') ||
    setweight(array_to_tsvector(user_tags), 'B')
) STORED;
CREATE INDEX items_search_tsv_idx ON items USING gin (search_tsv);
