import { NextResponse } from "next/server";

const API_URL = process.env.API_URL ?? "http://localhost:8080";

// Public pass-through: devices claim connect codes through the web ingress
// (the API is never exposed directly). No auth attached — the code IS the
// credential. The caller's IP is forwarded so the API's strict per-claim
// rate bucket throttles real clients, not the web container.
export async function POST(req: Request) {
  const body = await req.text();
  const headers = new Headers({ "content-type": "application/json" });
  const xff = req.headers.get("x-forwarded-for");
  if (xff) headers.set("x-forwarded-for", xff);
  const res = await fetch(`${API_URL}/device-links/claim`, {
    method: "POST",
    headers,
    body,
    cache: "no-store",
  });
  return new NextResponse(await res.text(), {
    status: res.status,
    headers: { "content-type": "application/json" },
  });
}
