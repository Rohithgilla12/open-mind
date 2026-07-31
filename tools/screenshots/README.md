# Screenshot generation

Regenerates the screenshots in `docs/screenshots/` (README) and
`docs/store/screenshots/` (Chrome Web Store listing).

Everything here runs against a **throwaway stack seeded with mock data**. It
never touches your real instance and never captures real saved items — the
published screenshots must not contain anyone's actual library.

## Why the data is seeded straight into Postgres

`seed.sql` inserts fully-enriched rows rather than saving URLs through the
pipeline. Two reasons:

- The stack runs `AI_PROVIDER=noop`, so the pipeline would produce no
  summaries, tags, or palettes — an empty-looking library.
- Fetching real pages would make the output non-deterministic and depend on
  third-party sites staying up.

The `palette` values are the genuine dominant colours of the generated images,
so the palette dots on each card are truthful rather than decorative.

## Prerequisites

- Docker (for the throwaway Postgres + api + web)
- Python 3 with Pillow — `pip install pillow`
- Node 20+, then `pnpm install` **in this directory** (it is deliberately
  outside the workspace) and `pnpm exec playwright install chromium`
- ImageMagick (`magick`) for the final resizing

## Run

```bash
cd tools/screenshots

# 1. Generate the abstract card art (deterministic; writes ./img)
python3 gen_images.py

# 2. Serve those images — the seeded lead_image_url values point here
python3 -m http.server 8899 --bind 127.0.0.1 --directory img &

# 3. Bring up the isolated stack (spare ports: db 5455, api 8788, web 3999)
cd ../..
docker compose -p openmind-shots \
  -f docker-compose.yml \
  -f tools/screenshots/compose.shots.yml \
  up -d --build

# 4. Seed
docker exec -i openmind-shots-db-1 psql -U openmind -d openmind \
  -v ON_ERROR_STOP=1 < tools/screenshots/seed.sql

# 5. Capture → tools/screenshots/out/*.png at 2560x1600
cd tools/screenshots && pnpm capture

# 6. Derive the two published sets
mkdir -p ../../docs/screenshots ../../docs/store/screenshots
for f in out/*.png; do n=$(basename "$f" .png)
  magick "$f" -resize 1600x1000 -strip -quality 86 "../../docs/screenshots/$n.jpg"
done
for n in the-mind reader search-colour desk drift; do
  magick "out/$n.png" -resize 1280x800! -strip "../../docs/store/screenshots/$n.png"
done

# 7. Tear down — this also drops the throwaway volumes
cd ../.. && docker compose -p openmind-shots down -v
pkill -f "http.server 8899"
```

## Notes

- **Chrome Web Store requires exactly 1280×800** (or 640×400), hence the `!`
  on that resize. Captures are taken at `deviceScaleFactor: 2` so the
  downscale stays sharp.
- The isolated stack uses the compose project name `openmind-shots`, giving it
  its own network and volumes. Your default-project instance is untouched.
- No DOM fixups are applied. The sidebar reads real data from `GET /account`,
  and token mode renders a neutral "Self-hosted", so nothing personal or
  fabricated ends up in a capture. (It used to hardcode a name and a fake
  storage meter, which the script had to rewrite before publishing.)
- The Places screenshot loads OpenStreetMap raster tiles, so it needs network
  access and keeps the OSM attribution visible in-frame (ODbL requires it).
- `out/`, `img/`, and `node_modules/` are git-ignored; only the derived images
  under `docs/` are committed.
