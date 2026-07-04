import { NextResponse } from "next/server";
import { apiFetch } from "../../../lib/api";

export async function POST(req: Request) {
  const body = await req.text();
  const res = await apiFetch("/lenses", { method: "POST", body }, req);
  return new NextResponse(await res.text(), {
    status: res.status,
    headers: { "content-type": "application/json" },
  });
}
