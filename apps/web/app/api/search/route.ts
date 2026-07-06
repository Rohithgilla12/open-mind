import { NextResponse } from "next/server";
import { apiFetch } from "../../../lib/api";

export async function GET(req: Request) {
  const search = new URL(req.url).search;
  const res = await apiFetch(`/search${search}`, undefined, req);
  return new NextResponse(res.body, {
    status: res.status,
    headers: { "content-type": res.headers.get("content-type") ?? "application/json" },
  });
}
