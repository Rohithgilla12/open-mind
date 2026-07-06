import { NextResponse } from "next/server";
import { apiFetch } from "../../../lib/api";

export async function GET(req: Request) {
  const search = new URL(req.url).search;
  const res = await apiFetch(`/items${search}`, undefined, req);
  return new NextResponse(res.body, {
    status: res.status,
    headers: { "content-type": res.headers.get("content-type") ?? "application/json" },
  });
}

export async function POST(req: Request) {
  const body = await req.text();
  const res = await apiFetch("/items", { method: "POST", body }, req);
  return new NextResponse(await res.text(), {
    status: res.status,
    headers: { "content-type": "application/json" },
  });
}
