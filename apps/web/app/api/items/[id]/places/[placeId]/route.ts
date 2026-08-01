import { NextResponse } from "next/server";
import { apiFetch } from "../../../../../../lib/api";

export async function DELETE(
  req: Request,
  { params }: { params: Promise<{ id: string; placeId: string }> },
) {
  const { id, placeId } = await params;
  try {
    const res = await apiFetch(`/items/${id}/places/${placeId}`, { method: "DELETE" }, req);
    return new NextResponse(null, { status: res.status });
  } catch (err) {
    console.error("place DELETE proxy failed", { itemId: id, placeId, err });
    return NextResponse.json({ error: "could not reach API" }, { status: 502 });
  }
}
