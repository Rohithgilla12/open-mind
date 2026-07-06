import { NextResponse } from "next/server";
import { apiFetch } from "../../../../lib/api";

// Same-origin proxy for the Drift action: forwards {keep} to the API with the
// cookie→bearer swap, passing status and body straight back to the client.
export async function POST(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const body = await req.text();
  const res = await apiFetch(`/drift/${id}`, { method: "POST", body }, req);
  return new NextResponse(res.body, {
    status: res.status,
    headers: { "content-type": res.headers.get("content-type") ?? "application/json" },
  });
}
