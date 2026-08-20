import { resolveColor } from "./colors";

/**
 * Query string for the live server search.
 *
 * `parse=true` lets the AI provider split free text into text + colour + type
 * filters, exactly as the server-rendered search does. On top of that, a query
 * that is itself a colour term is ALSO sent as `color=`, which is what the
 * colour chips do: /search fuses text and colour as soft rank signals via RRF
 * (see RunQuery in apps/api/internal/search/search.go), so sending both can
 * only widen the answer.
 *
 * That matters most on a `noop` AI provider, where nothing parses the query:
 * without an explicit colour, "cobalt" would reach Postgres as a literal word
 * search and return nothing, while the local index was matching it against
 * every extracted palette in the library.
 *
 * Only a whole-query colour term is forwarded. "cobalt print" is left to the
 * provider's parser, which is the thing that can actually tell which word was
 * meant as a colour.
 */
export function serverSearchParams(raw: string): URLSearchParams {
  const q = raw.trim();
  const params = new URLSearchParams({ q, parse: "true" });
  if (resolveColor(q)) params.set("color", q);
  return params;
}
