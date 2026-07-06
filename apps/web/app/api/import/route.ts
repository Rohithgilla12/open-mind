import { NextResponse } from "next/server";
import { apiFetch } from "../../../lib/api";

export async function POST(req: Request) {
  // Re-read the multipart form and hand FormData to fetch, which rebuilds a
  // valid multipart body + boundary. apiFetch leaves non-string bodies alone.
  const form = await req.formData();
  const res = await apiFetch("/import", { method: "POST", body: form }, req);
  return new NextResponse(await res.text(), {
    status: res.status,
    headers: { "content-type": "application/json" },
  });
}
