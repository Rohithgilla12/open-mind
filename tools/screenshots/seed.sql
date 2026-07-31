-- Mock corpus for README / store screenshots.
-- Inserted straight into the DB rather than through the enrichment pipeline:
-- with AI_PROVIDER=noop there would be no summaries, tags, or palettes, and the
-- point of these captures is to show an enriched library.
--
-- Palettes are the genuine dominant colours of each generated image (see
-- gen_images.py), so the palette dots on each card are truthful.
--
-- All rows belong to api.DevUserID (token mode's single user).

BEGIN;

\set uid '00000000-0000-0000-0000-000000000001'
\set img 'http://127.0.0.1:8899'

INSERT INTO users (id, email) VALUES (:'uid', 'you@example.com')
  ON CONFLICT (id) DO NOTHING;

DELETE FROM item_places WHERE user_id = :'uid';
DELETE FROM highlights WHERE user_id = :'uid';
DELETE FROM links WHERE user_id = :'uid';
DELETE FROM items WHERE user_id = :'uid';
DELETE FROM lenses WHERE user_id = :'uid';
DELETE FROM feeds WHERE user_id = :'uid';

-- ---------------------------------------------------------------- feeds
INSERT INTO feeds (id, user_id, url, title, site_url, last_polled_at, last_status) VALUES
 ('f0000000-0000-0000-0000-000000000001', :'uid', 'https://craigmod.com/feed.xml', 'Craig Mod', 'https://craigmod.com', now() - interval '18 minutes', 'ok'),
 ('f0000000-0000-0000-0000-000000000002', :'uid', 'https://www.robinsloan.com/feed.xml', 'Robin Sloan', 'https://www.robinsloan.com', now() - interval '42 minutes', 'ok'),
 ('f0000000-0000-0000-0000-000000000003', :'uid', 'https://increment.com/feed.xml', 'Increment', 'https://increment.com', now() - interval '2 hours', 'ok');

-- ---------------------------------------------------------------- lenses
INSERT INTO lenses (id, user_id, name, rule, created_at) VALUES
 ('e0000000-0000-0000-0000-000000000001', :'uid', 'Typography',   '{"q":"type OR typography OR lettering"}', now() - interval '40 days'),
 ('e0000000-0000-0000-0000-000000000002', :'uid', 'Deep blues',   '{"color":"#1B3FD1"}',                    now() - interval '31 days'),
 ('e0000000-0000-0000-0000-000000000003', :'uid', 'To cook',      '{"q":"recipe","types":["recipe"]}',      now() - interval '22 days'),
 ('e0000000-0000-0000-0000-000000000004', :'uid', 'Long reads',   '{"q":"essay OR long read"}',             now() - interval '9 days');

-- ---------------------------------------------------------------- items
INSERT INTO items (id, user_id, url, title, body, lead_image_url, summary, tags, user_tags, card_type, status, palette, created_at, pinned_at, last_drifted_at, feed_id, kept_at, tagged_location) VALUES

-- Articles
('a1000000-0000-0000-0000-000000000001', :'uid', 'https://craigmod.com/essays/fast_slow/',
 'Fast Software, Slow Software',
 E'Software that responds instantly feels like an extension of the body. Software that stutters feels like an argument.\n\nThe difference is rarely raw compute. It is a thousand decisions about what to do first, what to defer, and what to never do at all. A save that waits on a network round trip is a save that will be abandoned. A save that lands in eight milliseconds becomes a habit.\n\nSpeed is not a feature you add at the end. It is a constraint you accept at the beginning, and then defend, repeatedly, against every reasonable-sounding request to loosen it.\n\nThe interfaces I return to are the ones that never make me wait to think.',
 :'img' || '/cathedral.jpg',
 'An argument that perceived speed is a design constraint rather than an optimisation pass — and that latency budgets should be defended from the first commit.',
 '{software,performance,craft,essay}', '{reference}', 'article', 'enriched',
 '{"#1B3FD1","#17206B","#E4DDCD","#C24A2E"}', now() - interval '2 hours', now() - interval '1 hour', NULL, NULL, NULL, ''),

('a1000000-0000-0000-0000-000000000002', :'uid', 'https://www.robinsloan.com/notes/home-cooked-app/',
 'An app can be a home-cooked meal',
 E'Not everything has to scale. Some software is made for six people, or one, and that is a complete and honourable ambition.\n\nWe have lost the vocabulary for software made at the scale of a household. Everything is a startup or it is nothing. But a home-cooked app is a different category: it is made by someone who knows the eaters.\n\nThe code can be bad. The code can be embarrassing. It works for the six people, and the six people love it.',
 :'img' || '/linen.jpg',
 'A case for software written for a handful of known people — small, unscalable, and made with the same care as a meal cooked at home.',
 '{software,community,essay,craft}', '{}', 'article', 'enriched',
 '{"#E4DDCD","#F4F0E6","#A39C8B","#57534A"}', now() - interval '1 day', NULL, NULL, NULL, NULL, ''),

('a1000000-0000-0000-0000-000000000003', :'uid', 'https://increment.com/documentation/why-write-docs/',
 'The documentation you write is the product',
 E'Every interface has two surfaces: the one you can click and the one you have to read. Teams obsess over the first and resent the second.\n\nBut the docs are where a user forms their mental model. A confused sentence in a README costs more than a misaligned button.',
 '',
 'Argues that documentation is not support material but the primary surface through which users build a mental model of a system.',
 '{writing,documentation,craft}', '{work}', 'article', 'enriched',
 '{"#1B3FD1","#57534A","#E4DDCD"}', now() - interval '3 days', NULL, now() - interval '11 days', NULL, NULL, ''),

-- Quotes
('a1000000-0000-0000-0000-000000000010', :'uid', 'https://craigmod.com/essays/fast_slow/',
 'On latency as respect',
 E'A save that waits on a network round trip is a save that will be abandoned. A save that lands in eight milliseconds becomes a habit.',
 '',
 '',
 '{performance,craft}', '{favourite}', 'quote', 'enriched',
 '{"#E0B23A","#1C1A16","#F4F0E6"}', now() - interval '2 hours', now() - interval '30 minutes', NULL, NULL, NULL, ''),

('a1000000-0000-0000-0000-000000000011', :'uid', '',
 'Borges, on the library',
 E'I have always imagined that Paradise will be a kind of library.',
 '',
 '',
 '{books,libraries}', '{favourite}', 'quote', 'enriched',
 '{"#17206B","#1C1A16","#E4DDCD"}', now() - interval '6 days', NULL, now() - interval '20 days', NULL, NULL, ''),

-- Books
('a1000000-0000-0000-0000-000000000020', :'uid', 'https://openlibrary.org/works/OL27258W',
 'The Shape of Design',
 E'Frank Chimero on the craft of designing with intent — form, story, and the gap between what we make and why.',
 :'img' || '/kiln.jpg',
 'Chimero''s short, warm book on design as storytelling: less about tools, more about the intent behind a made thing.',
 '{design,books,craft}', '{to-reread}', 'book', 'enriched',
 '{"#C24A2E","#E0B23A","#F4F0E6","#1C1A16"}', now() - interval '9 days', now() - interval '4 days', NULL, NULL, NULL, ''),

('a1000000-0000-0000-0000-000000000021', :'uid', 'https://openlibrary.org/works/OL1725019W',
 'Thinking with Type',
 E'Ellen Lupton''s field guide to letters, text, and grid — the book most designers actually keep on the desk.',
 :'img' || '/brass.jpg',
 'A practical, opinionated guide to typography: anatomy of letterforms, text hierarchy, and grid systems.',
 '{typography,design,books,reference}', '{}', 'book', 'enriched',
 '{"#E0B23A","#C24A2E","#1C1A16","#F4F0E6"}', now() - interval '15 days', NULL, now() - interval '30 days', NULL, NULL, ''),

-- Recipes
('a1000000-0000-0000-0000-000000000030', :'uid', 'https://www.seriouseats.com/miso-butter-pasta',
 'Miso butter pasta, 12 minutes',
 E'White miso, cold butter, pasta water, black pepper. Emulsify off the heat or it splits.\n\nFinish with more pepper than feels correct.',
 :'img' || '/saffron.jpg',
 'A four-ingredient weeknight pasta: miso and cold butter emulsified with starchy pasta water, heavy on black pepper.',
 '{recipe,pasta,quick,dinner}', '{cook-soon}', 'recipe', 'enriched',
 '{"#E0B23A","#F4F0E6","#C24A2E","#2E7D5B"}', now() - interval '4 days', NULL, NULL, NULL, NULL, ''),

('a1000000-0000-0000-0000-000000000031', :'uid', 'https://www.kingarthurbaking.com/recipes/no-knead-focaccia',
 'Overnight focaccia',
 E'Mix at night, dimple in the morning, bake hot with far too much olive oil.',
 :'img' || '/ember.jpg',
 'A no-knead focaccia with an overnight cold ferment — mixed the night before, dimpled and baked the next day.',
 '{recipe,baking,bread}', '{cook-soon}', 'recipe', 'enriched',
 '{"#C24A2E","#1C1A16","#E0B23A","#F4F0E6"}', now() - interval '12 days', NULL, now() - interval '25 days', NULL, NULL, ''),

-- Products
('a1000000-0000-0000-0000-000000000040', :'uid', 'https://www.kaweco-pen.com/sport-classic',
 'Kaweco Sport, classic',
 E'Pocket fountain pen, octagonal barrel, closes to 105mm.',
 :'img' || '/moss.jpg',
 'A compact octagonal fountain pen that closes to pocket length — the standard recommendation for a first fountain pen.',
 '{stationery,pens,edc}', '{wishlist}', 'product', 'enriched',
 '{"#2E7D5B","#1F5A41","#E4DDCD","#E0B23A"}', now() - interval '7 days', NULL, NULL, NULL, NULL, ''),

-- Images
('a1000000-0000-0000-0000-000000000050', :'uid', 'https://www.are.na/block/colour-study-ochre',
 'Colour study — ochre and slate',
 '', :'img' || '/clay.jpg',
 'A colour study pairing warm ochre against cool slate, saved as a palette reference.',
 '{colour,reference,inspiration}', '{palettes}', 'image', 'enriched',
 '{"#A39C8B","#C24A2E","#EBE5D7","#1C1A16"}', now() - interval '5 days', now() - interval '2 days', NULL, NULL, NULL, ''),

('a1000000-0000-0000-0000-000000000051', :'uid', 'https://www.are.na/block/deep-blue-strata',
 'Deep blue strata',
 '', :'img' || '/dusk.jpg',
 'Layered blues, saved for the gradient — a reference for cool-toned interface backgrounds.',
 '{colour,gradient,inspiration}', '{palettes}', 'image', 'enriched',
 '{"#17206B","#1B3FD1","#C24A2E","#EBE5D7"}', now() - interval '11 days', NULL, now() - interval '18 days', NULL, NULL, ''),

('a1000000-0000-0000-0000-000000000052', :'uid', 'https://www.are.na/block/harbour-green',
 'Harbour, early',
 '', :'img' || '/harbour.jpg',
 'Cool green and cobalt in the same frame — kept as a colour pairing reference.',
 '{colour,inspiration}', '{}', 'image', 'enriched',
 '{"#1B3FD1","#2E7D5B","#F4F0E6","#E0B23A"}', now() - interval '20 days', NULL, NULL, NULL, NULL, ''),

-- Notes
('a1000000-0000-0000-0000-000000000060', :'uid', '',
 'Why I keep a commonplace book',
 E'Not to remember everything. To make a place where the things I noticed can find each other.\n\nThe value is not in the saving. It is in the collision six months later.',
 '', '',
 '{notes,thinking}', '{}', 'note', 'enriched',
 '{"#FBF4D8","#E0B23A","#3A3320"}', now() - interval '8 hours', now() - interval '5 hours', NULL, NULL, NULL, ''),

('a1000000-0000-0000-0000-000000000061', :'uid', '',
 'Reading queue, winter',
 E'— Thinking with Type (reread the grid chapter)\n— Chimero, The Shape of Design\n— That Increment piece on docs\n— Borges, Ficciones',
 '', '',
 '{notes,reading}', '{}', 'note', 'enriched',
 '{"#FBF4D8","#E0B23A","#9A8A3A"}', now() - interval '2 days', NULL, NULL, NULL, NULL, ''),

-- Video with places (reel → places extraction)
('a1000000-0000-0000-0000-000000000070', :'uid', 'https://www.instagram.com/reel/coffee-kl-crawl/',
 'Three coffee bars in one afternoon, KL',
 E'Bangsar to Damansara: filter at the first, a cortado at the second, and a very serious siphon at the third.',
 :'img' || '/indigo.jpg',
 'A short city guide reel covering three speciality coffee bars in Kuala Lumpur, with each stop named in the caption.',
 '{coffee,travel,kualalumpur,video}', '{places}', 'video', 'enriched',
 '{"#17206B","#1C1A16","#1B3FD1","#E4DDCD"}', now() - interval '16 hours', NULL, NULL, NULL, NULL, 'Kuala Lumpur, Malaysia'),

-- Tweet / post
('a1000000-0000-0000-0000-000000000080', :'uid', 'https://twitter.com/example/status/1',
 'On tools that outlive their vendor',
 E'The best thing you can say about a tool is that you could still read your own data if the company vanished tomorrow.',
 '', '',
 '{software,ownership}', '{}', 'tweet', 'enriched',
 '{"#1B3FD1","#2E7D5B","#F4F0E6"}', now() - interval '3 days', NULL, NULL, NULL, NULL, ''),

-- Unkept feed-river items (searchable, not yet in the Mind)
('a1000000-0000-0000-0000-000000000090', :'uid', 'https://craigmod.com/roden/ridgeline-190/',
 'Ridgeline 190: Walking the old post road',
 E'Twenty kilometres of cedar and rain, and a vending machine exactly where it was needed.',
 :'img' || '/slate.jpg',
 'A walking dispatch along an old Japanese post road — cedar forest, rain, and the logistics of a long day on foot.',
 '{walking,japan,essay}', '{}', 'article', 'enriched',
 '{"#57534A","#A39C8B","#E4DDCD","#1B3FD1"}', now() - interval '5 hours', NULL, NULL, 'f0000000-0000-0000-0000-000000000001', NULL, ''),

('a1000000-0000-0000-0000-000000000091', :'uid', 'https://www.robinsloan.com/lab/spring-and-autumn/',
 'Spring and autumn, and the media they suit',
 E'Some formats only make sense in a particular season of attention.',
 '',
 'A short note on matching creative formats to seasons of attention rather than to platforms.',
 '{media,essay}', '{}', 'article', 'enriched',
 '{"#E4DDCD","#57534A","#1B3FD1"}', now() - interval '9 hours', NULL, NULL, 'f0000000-0000-0000-0000-000000000002', NULL, '');

-- ------------------------------------------------- places (map view)
INSERT INTO item_places (user_id, item_id, name, hint, address, lat, lng, source) VALUES
 (:'uid', 'a1000000-0000-0000-0000-000000000070', 'VCR Bangsar',        'filter coffee', 'Jalan Telawi, Bangsar, Kuala Lumpur',            3.1319, 101.6708, 'caption'),
 (:'uid', 'a1000000-0000-0000-0000-000000000070', 'Feeka Coffee',       'cortado',       'Jalan Mesui, Bukit Bintang, Kuala Lumpur',       3.1478, 101.7089, 'caption'),
 (:'uid', 'a1000000-0000-0000-0000-000000000070', 'Artisan Roast TTDI', 'siphon',        'Jalan Wan Kadir, TTDI, Kuala Lumpur',           3.1385, 101.6301, 'caption');

-- ------------------------------------------------- highlight (quote from article)
INSERT INTO highlights (user_id, source_item_id, quote_item_id, exact, prefix, suffix, offset_hint) VALUES
 (:'uid',
  'a1000000-0000-0000-0000-000000000001',
  'a1000000-0000-0000-0000-000000000010',
  'A save that waits on a network round trip is a save that will be abandoned. A save that lands in eight milliseconds becomes a habit.',
  'what to never do at all. ', ' Speed is not a feature', 312);

-- ------------------------------------------------- links (related items)
INSERT INTO links (user_id, a_item, b_item) VALUES
 (:'uid', 'a1000000-0000-0000-0000-000000000001', 'a1000000-0000-0000-0000-000000000002'),
 (:'uid', 'a1000000-0000-0000-0000-000000000020', 'a1000000-0000-0000-0000-000000000021');

COMMIT;

SELECT card_type, count(*) FROM items WHERE user_id = '00000000-0000-0000-0000-000000000001' GROUP BY 1 ORDER BY 1;
