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
          style={{ display: "block", color: "inherit", textDecoration: "none" }}
        >
          <ItemCard item={item} />
        </Link>
      ))}
    </div>
  );
}
