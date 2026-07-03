CREATE TABLE assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    item_id uuid REFERENCES items(id) ON DELETE CASCADE,
    content_type text NOT NULL,
    byte_size bigint NOT NULL,
    original_filename text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX assets_user_idx ON assets (user_id);
