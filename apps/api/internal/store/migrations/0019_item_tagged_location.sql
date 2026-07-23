-- Tagged location parsed from a social-video page's inline JSON (Phase 4).
-- Empty string = no tag (matches the item_places hint/address convention).
ALTER TABLE items ADD COLUMN tagged_location text NOT NULL DEFAULT '';
