CREATE TABLE links (
    slug       text        PRIMARY KEY,
    url        text        NOT NULL CHECK (length(url) <= 2048),
    custom     boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- "Recent links" listing: newest first with keyset pagination.
CREATE INDEX links_created_at_idx ON links (created_at DESC, slug);
