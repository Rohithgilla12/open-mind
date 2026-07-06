-- Stem tags at index time so they match websearch_to_tsquery('english', ...)
-- on the query side: array_to_tsvector indexes literal lexemes, so any
-- English-stemmable tag ("favourite" → query lexeme "favourit") never matched.
--
-- array_to_string() is marked STABLE (not IMMUTABLE) in Postgres, because for
-- generic anyarray input its element output could be locale-dependent. For
-- our fixed text[] columns that never applies, so we wrap it in a trivial
-- IMMUTABLE SQL function to satisfy the generated-column requirement.
CREATE OR REPLACE FUNCTION items_tags_to_text(tags text[]) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT array_to_string(tags, ' ');
$$;

DROP INDEX items_search_tsv_idx;
ALTER TABLE items DROP COLUMN search_tsv;
ALTER TABLE items ADD COLUMN search_tsv tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('english'::regconfig, title), 'A') ||
    setweight(to_tsvector('english'::regconfig, summary), 'B') ||
    setweight(to_tsvector('english'::regconfig, items_tags_to_text(tags)), 'B') ||
    setweight(to_tsvector('english'::regconfig, left(body, 100000)), 'C') ||
    setweight(to_tsvector('english'::regconfig, items_tags_to_text(user_tags)), 'B')
) STORED;
CREATE INDEX items_search_tsv_idx ON items USING gin (search_tsv);
