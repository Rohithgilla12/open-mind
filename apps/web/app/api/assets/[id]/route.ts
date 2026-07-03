import { NextResponse } from "next/server";
import { apiFetch } from "../../../../lib/api";

export async function GET(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/assets/${id}`, undefined, req);
  if (!res.ok) {
    return new NextResponse(null, { status: res.status });
  }
  const headers = new Headers();
  const contentType = res.headers.get("content-type");
  if (contentType) headers.set("content-type", contentType);
  headers.set("X-Content-Type-Options", "nosniff");
  return new NextResponse(res.body, { status: res.status, headers });
}
