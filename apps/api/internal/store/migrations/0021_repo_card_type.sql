-- Code-forge repository URLs get their own card type instead of being filed as
-- articles. The rule for new saves lives in Go (enrich.Classify); this function
-- is its SQL twin, used for the one-off backfill below and kept afterwards so
-- internal/store/repo_url_test.go can assert the two still agree.
--
-- Two predicates rather than one regex with a negative lookahead: each half
-- maps to one half of the Go rule (>=2 path segments; first segment not a
-- reserved host route), which keeps them reviewable side by side.
--
-- The authority prefix `([^/?#@]*@)?(www\.)?HOST(:[0-9]+)?` mirrors what Go's
-- url.Parse + Hostname() strip before enrich.Classify ever sees the host:
-- userinfo (a Bitbucket HTTPS clone URL is literally `https://user@bitbucket
-- .org/workspace/project`) and an explicit port. Without it those rows parse
-- as non-matches here while Go calls them "repo", and since this migration
-- runs once, the backfill would silently and permanently skip them.
--
-- `/+` (not `/`) between path segments tolerates doubled slashes from naive
-- URL concatenation (share sheets, bookmarklets), matching Go's behaviour of
-- dropping empty path segments before counting.
--
-- WARNING: this migration is already in schema_migrations on deployed
-- databases, so editing the CREATE OR REPLACE below to add a fifth host is a
-- no-op there — new saves would classify correctly but existing rows for that
-- host would never be backfilled. Adding a host requires a NEW migration that
-- re-`CREATE OR REPLACE`s this function AND re-runs the UPDATE below.
CREATE OR REPLACE FUNCTION item_url_is_repo(url text) RETURNS boolean
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT url ~* '^https?://([^/?#@]*@)?(www\.)?(github\.com|gitlab\.com|codeberg\.org|bitbucket\.org)(:[0-9]+)?/+[^/?#]+/+[^/?#]+'
       AND url !~* '^https?://([^/?#@]*@)?(www\.)?(github\.com|gitlab\.com|codeberg\.org|bitbucket\.org)(:[0-9]+)?/+(about|apps|collections|contact|enterprise|explore|features|join|login|marketplace|notifications|orgs|pricing|pulls|readme|security|settings|sponsors|topics|trending|-)(/|\?|#|$)';
$$;

-- Scoped to 'article' so the backfill cannot overwrite an image or PDF
-- classification that the pipeline determined by other means.
UPDATE items SET card_type = 'repo'
WHERE card_type = 'article' AND item_url_is_repo(url);
