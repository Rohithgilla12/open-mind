import type { paths } from "@openmind/api-client";

export type Item =
  paths["/items"]["get"]["responses"]["200"]["content"]["application/json"][number];

export type SearchResult =
  paths["/search"]["get"]["responses"]["200"]["content"]["application/json"][number];
