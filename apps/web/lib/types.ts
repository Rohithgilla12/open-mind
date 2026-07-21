import type { paths } from "@openmind/api-client";

export type Item =
  paths["/items"]["get"]["responses"]["200"]["content"]["application/json"][number];

export type SearchResponse =
  paths["/search"]["get"]["responses"]["200"]["content"]["application/json"];

export type SearchResult = NonNullable<SearchResponse["results"]>[number];

export type UnderstoodQuery = NonNullable<SearchResponse["understood"]>;

export type ItemDetail =
  paths["/items/{id}"]["get"]["responses"]["200"]["content"]["application/json"];

export type Lens =
  paths["/lenses"]["get"]["responses"]["200"]["content"]["application/json"][number];

export type LensRule = Lens["rule"];

export type CreateLensRequest =
  paths["/lenses"]["post"]["requestBody"]["content"]["application/json"];

export type ImportResult =
  paths["/import"]["post"]["responses"]["200"]["content"]["application/json"];

export type Feed =
  paths["/feeds"]["get"]["responses"]["200"]["content"]["application/json"][number];

export type DriftResponse =
  paths["/drift"]["get"]["responses"]["200"]["content"]["application/json"];

export type ApiKey =
  paths["/api-keys"]["get"]["responses"]["200"]["content"]["application/json"][number];

export type ApiKeyCreated =
  paths["/api-keys"]["post"]["responses"]["201"]["content"]["application/json"];

export type DeviceLinkCreated =
  paths["/device-links"]["post"]["responses"]["201"]["content"]["application/json"];

export type Settings =
  paths["/settings"]["get"]["responses"]["200"]["content"]["application/json"];

export type PatchSettingsRequest =
  paths["/settings"]["patch"]["requestBody"]["content"]["application/json"];

export type Highlight =
  paths["/items/{id}/highlights"]["get"]["responses"]["200"]["content"]["application/json"][number];

export type CreateHighlightRequest =
  paths["/items/{id}/highlights"]["post"]["requestBody"]["content"]["application/json"];

export type CreateHighlightResponse =
  paths["/items/{id}/highlights"]["post"]["responses"]["201"]["content"]["application/json"];

export type RelatedItem =
  paths["/items/{id}/related"]["get"]["responses"]["200"]["content"]["application/json"][number];

// One element of GET /places, straight from the OpenAPI contract.
export type MapPlace =
  paths["/places"]["get"]["responses"]["200"]["content"]["application/json"][number];
