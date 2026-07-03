import { NextResponse } from "next/server";
import { apiFetch } from "../../../../lib/api";

export async function GET(req: Request) {
  try {
    const res = await apiFetch("/items?limit=1", undefined, req);
    if (res.ok) return NextResponse.json({ ok: true });
    if (res.status === 429) return NextResponse.json({ error: "rate limited" }, { status: 429 });
    return NextResponse.json({ error: "invalid token" }, { status: 401 });
  } catch {
    return NextResponse.json({ error: "api unreachable" }, { status: 502 });
  }
}
