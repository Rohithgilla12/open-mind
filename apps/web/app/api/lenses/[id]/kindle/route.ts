import { NextResponse } from "next/server";
import { apiFetch } from "../../../../../lib/api";

// Same-origin proxy for the Send-digest-to-Kindle action: body-less POST,
// cookie→bearer swap, status and body passed straight back to the client.
export async function POST(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/lenses/${id}/kindle`, { method: "POST" }, req);
  return new NextResponse(res.body, {
    status: res.status,
    headers: { "content-type": res.headers.get("content-type") ?? "application/json" },
  });
}
