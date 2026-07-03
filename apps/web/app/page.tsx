import type { paths } from "@openmind/api-client";
import { tokens } from "@openmind/ui";
import { apiFetch } from "../lib/api";

type Item =
  paths["/items"]["get"]["responses"]["200"]["content"]["application/json"][number];

async function getItems(): Promise<Item[]> {
  try {
    const res = await apiFetch("/items");
    if (!res.ok) return [];
    return ((await res.json()) as Item[]) ?? [];
  } catch {
    // Enrichment/API may be down; render an empty state rather than failing the build.
    return [];
  }
}

export default async function Page() {
  const items = await getItems();

  return (
    <main style={{ backgroundColor: tokens.color.paper, minHeight: "100vh", padding: "2rem" }}>
      <h1 style={{ color: tokens.color.ink, fontFamily: tokens.font.sans }}>Openmind</h1>
      <ul>
        {items.map((item) => (
          <li key={item.id} style={{ color: tokens.color.ink, fontFamily: tokens.font.sans }}>
            {item.title ?? item.url}
          </li>
        ))}
      </ul>
    </main>
  );
}
