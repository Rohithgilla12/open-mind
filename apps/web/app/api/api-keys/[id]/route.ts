import { NextResponse } from "next/server";
import { apiFetch } from "../../../../lib/api";

export async function DELETE(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/api-keys/${id}`, { method: "DELETE" }, req);
  return new NextResponse(null, { status: res.status });
}
