import { NextResponse } from "next/server";
import { apiFetch } from "../../../../../lib/api";

export async function GET(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const url = new URL(req.url);
  const qs = url.searchParams.toString();
  const path = qs ? `/lenses/${id}/items?${qs}` : `/lenses/${id}/items`;
  try {
    const res = await apiFetch(path, undefined, req);
    return new NextResponse(await res.text(), {
      status: res.status,
      headers: { "content-type": "application/json" },
    });
  } catch (err) {
    console.error("lenses items GET proxy failed", { lensId: id, err });
    return NextResponse.json({ error: "could not reach API" }, { status: 502 });
  }
}
