import "server-only";
import { cookies } from "next/headers";

const API_URL = process.env.API_URL ?? "http://localhost:8080";

export async function apiFetch(path: string, init?: RequestInit, req?: Request): Promise<Response> {
  let token = (await cookies()).get("om_token")?.value;
  const header = req?.headers.get("authorization");
  if (header?.startsWith("Bearer ")) token = header.slice(7);
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  // Only force JSON for string bodies. FormData/streams (multipart uploads)
  // must keep their own content-type (incl. the multipart boundary).
  if (typeof init?.body === "string") headers.set("content-type", "application/json");
  return fetch(`${API_URL}${path}`, { ...init, headers, cache: "no-store" });
}
