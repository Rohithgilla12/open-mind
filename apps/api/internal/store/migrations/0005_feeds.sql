CREATE TABLE feeds (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    url text NOT NULL,
    title text NOT NULL DEFAULT '',
    site_url text NOT NULL DEFAULT '',
    last_polled_at timestamptz,
    last_status text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, url)
);
CREATE INDEX feeds_user_idx ON feeds (user_id);
