-- The list sort key gained an id tiebreaker, so the index has to carry it for
-- the keyset seek to stay index-only.
--
-- CONCURRENTLY is deliberately absent: migrate.go runs each file inside a
-- transaction, and CREATE INDEX CONCURRENTLY cannot run in one.
CREATE INDEX items_user_created_id_idx ON items (user_id, created_at DESC, id DESC);

-- items_user_created_idx (0001_init.sql) is a strict prefix of the index above,
-- so every query it served is still served. Keeping both would pay the write
-- cost twice for nothing.
DROP INDEX items_user_created_idx;
