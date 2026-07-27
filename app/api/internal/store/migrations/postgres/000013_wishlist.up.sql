-- Wishlist: a shopping/buy list of things the user plans to purchase, with an
-- estimated price, a target month to buy (grouped so they can check out after
-- payroll), a priority, and a reference/marketplace link. Scoped to a user and
-- project, mirroring the bucket-list table.
CREATE TABLE wishlist_items (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    project_id      BIGINT NOT NULL DEFAULT 0,
    name            TEXT NOT NULL,
    estimated_price BIGINT NOT NULL DEFAULT 0,   -- whole currency units (e.g. IDR)
    buy_month       TEXT NOT NULL DEFAULT '',     -- target month "YYYY-MM"; '' when undecided
    priority        TEXT NOT NULL DEFAULT 'medium',
    link            TEXT NOT NULL DEFAULT '',
    note            TEXT NOT NULL DEFAULT '',
    done            BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    done_at         TIMESTAMPTZ
);
CREATE INDEX idx_wishlist_items_user ON wishlist_items(user_id, done);
CREATE INDEX idx_wishlist_items_project ON wishlist_items(project_id);
CREATE INDEX idx_wishlist_items_month ON wishlist_items(user_id, buy_month);
