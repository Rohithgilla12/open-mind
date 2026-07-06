CREATE TABLE links (
    user_id    uuid NOT NULL REFERENCES users(id),
    a_item     uuid NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    b_item     uuid NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, a_item, b_item),
    CHECK (a_item < b_item)
);
CREATE INDEX links_b_idx ON links (user_id, b_item);
