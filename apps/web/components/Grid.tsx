import { tokens } from "@openmind/ui";
import type { Item } from "../lib/types";
import { ItemCard } from "./ItemCard";

export function Grid({ items }: { items: Item[] }) {
  if (items.length === 0) {
    return (
      <p
        style={{
          fontFamily: tokens.font.sans,
          color: tokens.color.ink,
          opacity: 0.6,
          marginTop: "2rem",
        }}
      >
        Nothing here yet — drop a link or a thought above.
      </p>
    );
  }

  return (
    <div className="grid">
      {items.map((item) => (
        <ItemCard key={item.id} item={item} />
      ))}
    </div>
  );
}
