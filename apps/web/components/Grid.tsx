import { tokens } from "@openmind/ui";
import type { Item } from "../lib/types";
import { ItemCard } from "./ItemCard";
import { CardLink } from "./CardLink";

/**
 * `morph` opts a grid into the Mind's live-search view transitions by giving
 * every card a `view-transition-name`, so a card that survives a filter change
 * glides to its new column instead of being replaced by a copy of itself.
 *
 * It is a prop rather than the default because a named element becomes a
 * stacking context and a containing block, and the card's stretched-link
 * overlay sits in a carefully ordered stack (see .card-wrap in globals.css) —
 * grids that never transition have no reason to take that on.
 */
export function Grid({
  items,
  colorActive,
  morph,
}: {
  items: Item[];
  colorActive?: boolean;
  morph?: boolean;
}) {
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
        {colorActive
          ? "No saves match that colour yet — colours come from each save's palette, so try a warmer or cooler shade."
          : "Nothing gathered yet — drop a link or a thought above."}
      </p>
    );
  }

  return (
    <div className="mind-col">
      {items.map((item) => (
        <article
          key={item.id}
          className="card-wrap"
          // Prefixed because a view-transition-name is a CSS custom-ident and
          // an item id can begin with a digit.
          style={morph ? { viewTransitionName: `card-${item.id}` } : undefined}
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
                pointerEvents: "none",
              }}
            />
          ) : null}
          <ItemCard item={item} />
          <CardLink href={`/item/${item.id}`} label={item.title ?? "Open item"} />
        </article>
      ))}
    </div>
  );
}
