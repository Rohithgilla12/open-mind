import { flushSync } from "react-dom";

/**
 * Same-document view transitions for the Mind's result grid.
 *
 * The grid is a CSS column masonry, so filtering moves cards between columns.
 * Applied plainly that reads as a snap; wrapped in a view transition the cards
 * glide from where they were to where they belong, which is the difference
 * between "the list changed" and "these are the same saves, rearranged".
 *
 * This is the same-document API (`document.startViewTransition`), supported in
 * Chrome, Edge and Safari, and it needs no React canary: React 19.2 stable does
 * not export `<ViewTransition>`, so the DOM API plus a `flushSync` is the whole
 * mechanism. Where it is missing — Firefox today — the update simply applies,
 * which is the instant behaviour we wanted anyway.
 */
type ViewTransition = { finished: Promise<void> };

type WithVT = Document & { startViewTransition?: (cb: () => void) => ViewTransition };

/**
 * One transition at a time. Starting a second while one runs makes the browser
 * skip the first to its end state — correct, but it reads as a stutter, and a
 * fast typist would trigger it on every keystroke. Holding the flag means the
 * first change glides, changes arriving mid-flight apply instantly, and the
 * settle after the last keystroke glides again.
 */
let inFlight = false;

export function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/**
 * Apply `update` inside a view transition where that is both supported and
 * wanted, and directly otherwise. `update` must be the state write itself:
 * `flushSync` forces React to commit it before the browser takes its "after"
 * snapshot.
 */
export function morph(update: () => void): void {
  const doc = typeof document === "undefined" ? null : (document as WithVT);
  if (!doc?.startViewTransition || inFlight || prefersReducedMotion()) {
    update();
    return;
  }
  inFlight = true;
  const transition = doc.startViewTransition(() => {
    flushSync(update);
  });
  void transition.finished.catch(() => {}).then(() => {
    inFlight = false;
  });
}
