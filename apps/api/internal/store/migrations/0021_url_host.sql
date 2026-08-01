-- Extract lowercased host from a URL-ish string; strip leading www.
-- Empty / unparseable → NULL so domain filters never false-match notes/quotes.
CREATE OR REPLACE FUNCTION items_url_host(u text) RETURNS text AS $$
  SELECT NULLIF(
    regexp_replace(
      lower(
        substring(
          u
          from '(?i)^(?:[a-z][a-z0-9+.-]*:)?(?://)?(?:[^@/\s]+@)?([^:/?\s#]+)'
        )
      ),
      '^www\.',
      ''
    ),
    ''
  );
$$ LANGUAGE SQL IMMUTABLE PARALLEL SAFE;

ALTER TABLE items
  ADD COLUMN url_host text
  GENERATED ALWAYS AS (items_url_host(url)) STORED;

CREATE INDEX items_url_host_idx ON items (user_id, url_host)
  WHERE url_host IS NOT NULL;
