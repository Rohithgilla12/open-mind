import createFetchClient from "openapi-fetch";
import type { paths } from "./schema";

export function createClient(baseUrl: string) {
  return createFetchClient<paths>({ baseUrl });
}
export type { paths };
