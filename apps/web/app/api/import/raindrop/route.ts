import { NextResponse } from "next/server";
import { apiFetch } from "../../../../lib/api";

export async function POST(req: Request) {
  // Forward the JSON body as-is; apiFetch sets content-type for string bodies.
  const body = await req.text();
  const res = await apiFetch("/import/raindrop", { method: "POST", body }, req);
  return new NextResponse(await res.text(), {
    status: res.status,
    headers: { "content-type": "application/json" },
  });
}
