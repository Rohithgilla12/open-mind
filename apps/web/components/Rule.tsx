import { tokens } from "@openmind/ui";

const { color } = tokens;

/**
 * The hairline that separates sections of the item rail. A component rather
 * than an inline div so a client section can render its own leading rule and
 * drop it when the section empties out.
 */
export function Rule() {
  return <div style={{ height: 1, background: color.hairline, margin: "18px 0" }} />;
}
