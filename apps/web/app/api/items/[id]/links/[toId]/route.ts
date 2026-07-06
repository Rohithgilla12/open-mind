import { NextResponse } from "next/server";
import { apiFetch } from "../../../../../../lib/api";

export async function DELETE(
  req: Request,
  { params }: { params: Promise<{ id: string; toId: string }> },
) {
  const { id, toId } = await params;
  const res = await apiFetch(`/items/${id}/links/${toId}`, { method: "DELETE" }, req);
  return new NextResponse(null, { status: res.status });
}
