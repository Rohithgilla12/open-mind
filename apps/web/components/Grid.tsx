import Link from "next/link";
import { tokens } from "@openmind/ui";
import type { Item } from "../lib/types";
import { ItemCard } from "./ItemCard";

export function Grid({ items }: { items: Item[] }) {
  if (items.length === 0) {
    return (
      <p
        style={{
          fontFamily: tokens.font.quote,
          fontStyle: "italic",
          fontSize: "1.25rem",
          color: tokens.color.inkMuted,
          marginTop: "2rem",
        }}
      >
        Nothing gathered yet — drop a link or a thought above.
      </p>
    );
  }

  return (
    <div className="mind-col">
      {items.map((item) => (
        <Link
          key={item.id}
          href={`/item/${item.id}`}
          style={{ display: "block", position: "relative", color: "inherit", textDecoration: "none" }}
        >
          {item.pinnedAt ? (
            <span
              aria-label="On desk"
              title="On desk"
              style={{
                position: "absolute",
                top: 10,
                right: 10,
                zIndex: 1,
                width: 8,
                height: 8,
                borderRadius: "50%",
                background: tokens.color.gold,
                boxShadow: `0 0 0 2px ${tokens.color.cardSurface}`,
              }}
            />
          ) : null}
          <ItemCard item={item} />
        </Link>
      ))}
    </div>
  );
}
