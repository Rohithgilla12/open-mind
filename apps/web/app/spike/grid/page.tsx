import { tokens } from "@openmind/ui";
import type { Item } from "../../../lib/types";
import { Grid } from "../../../components/Grid";

// Synthetic perf spike page (FSX masonry spike). No API, no auth — deterministic
// seeded data so the measurement reflects production rendering (real ItemCard +
// real .grid CSS) without any network variance. See docs/research.md.

// Small deterministic PRNG (mulberry32) so runs are reproducible.
function mulberry32(seed: number): () => number {
  let a = seed;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const CARD_TYPES = [
  "article",
  "note",
  "tweet",
  "image",
  "video",
  "product",
  "book",
  "quote",
  "recipe",
] as const;

const LOREM =
  "The commonplace book is a personal knowledge store where fragments of thought accumulate over time and resurface unexpectedly when you least expect them to but most need them. ".repeat(
    6,
  );

function makeItems(n: number): Item[] {
  const rand = mulberry32(0x5eed);
  const items: Item[] = [];
  for (let i = 0; i < n; i++) {
    const cardType = CARD_TYPES[Math.floor(rand() * CARD_TYPES.length)];
    const hasImage = cardType === "image" || cardType === "video" || rand() < 0.4;
    // Vary image height so masonry actually staggers columns.
    const imgH = 200 + Math.floor(rand() * 400);
    const leadImageUrl = hasImage
      ? `https://picsum.photos/seed/${i}/400/${imgH}`
      : undefined;
    // Vary text length across cards.
    const textLen = 40 + Math.floor(rand() * (LOREM.length - 40));
    const summary =
      cardType === "note" || cardType === "tweet" || rand() < 0.7
        ? LOREM.slice(0, textLen)
        : undefined;

    items.push({
      id: `synthetic-${i}`,
      url: `https://example.com/post/${i}`,
      title: cardType === "image" && rand() < 0.5 ? undefined : `Synthetic card ${i} — ${cardType}`,
      summary,
      leadImageUrl,
      tags: [],
      cardType,
      status: rand() < 0.15 ? "pending" : "enriched",
      createdAt: new Date(2026, 0, 1 + (i % 180)).toISOString(),
    });
  }
  return items;
}

export default async function SpikeGridPage({
  searchParams,
}: {
  searchParams: Promise<{ n?: string }>;
}) {
  const { n } = await searchParams;
  const count = Math.min(Math.max(Number(n) || 500, 1), 5000);
  const items = makeItems(count);

  return (
    <main style={{ maxWidth: 1200, margin: "0 auto", padding: "2rem 1.5rem" }}>
      <h1
        style={{
          fontFamily: tokens.font.sans,
          fontSize: "1.2rem",
          fontWeight: 600,
          color: tokens.color.ink,
          margin: "0 0 1rem",
        }}
      >
        Masonry perf spike — {count} synthetic cards
      </h1>
      <Grid items={items} />
    </main>
  );
}
