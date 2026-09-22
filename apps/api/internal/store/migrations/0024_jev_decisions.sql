-- Shadow (and later live) judgments from TypeSafe Jev. item_id is UUID to match
-- items.id; user_id scopes every read/write. Phase 1 only inserts rows — never
-- mutates items.
CREATE TABLE jev_decisions (
    id           bigserial PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id),
    item_id      uuid REFERENCES items(id) ON DELETE SET NULL,
    surface      text NOT NULL,        -- 'capture' | 'rerank' | 'drift'
    model        text NOT NULL,        -- versioned ID from the response (or pin)
    questions_v  text NOT NULL,        -- QuestionsVersion from questions.go
    answers      jsonb NOT NULL,       -- full answers payload ({} when skipped)
    action       text NOT NULL,        -- shadow | skipped | applied | suggested
    user_verdict text,                 -- kept | changed | removed (backfilled later)
    latency_ms   int,
    input_tokens int,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX jev_decisions_user_item_idx ON jev_decisions (user_id, item_id);
CREATE INDEX jev_decisions_user_surface_idx ON jev_decisions (user_id, surface, created_at DESC);

-- At most one capture decision per item so re-running enrich is a no-op for
-- the shadow log (idempotent for Phase 1 dogfooding).
CREATE UNIQUE INDEX jev_decisions_capture_item_uidx
    ON jev_decisions (user_id, item_id)
    WHERE surface = 'capture' AND item_id IS NOT NULL;
