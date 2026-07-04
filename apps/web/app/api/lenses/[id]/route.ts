import { NextResponse } from "next/server";
import { apiFetch } from "../../../../lib/api";

export async function PATCH(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const body = await req.text();
  const res = await apiFetch(`/lenses/${id}`, { method: "PATCH", body }, req);
  return new NextResponse(await res.text(), {
    status: res.status,
    headers: { "content-type": "application/json" },
  });
}

export async function DELETE(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/lenses/${id}`, { method: "DELETE" }, req);
  return new NextResponse(null, { status: res.status });
}
