CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    url text NOT NULL,
    title text NOT NULL DEFAULT '',
    body text NOT NULL DEFAULT '',
    lead_image_url text NOT NULL DEFAULT '',
    summary text NOT NULL DEFAULT '',
    tags text[] NOT NULL DEFAULT '{}',
    card_type text NOT NULL DEFAULT 'article',
    status text NOT NULL DEFAULT 'pending',
    search_tsv tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english'::regconfig, title), 'A') ||
        setweight(to_tsvector('english'::regconfig, summary), 'B') ||
        setweight(array_to_tsvector(tags), 'B') ||
        setweight(to_tsvector('english'::regconfig, left(body, 100000)), 'C')
    ) STORED,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX items_user_created_idx ON items (user_id, created_at DESC);
CREATE INDEX items_search_tsv_idx ON items USING gin (search_tsv);

CREATE TABLE item_embeddings (
    item_id uuid PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id),
    embedding vector(768) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX item_embeddings_user_idx ON item_embeddings (user_id);
