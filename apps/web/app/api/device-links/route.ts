import { NextResponse } from "next/server";
import { apiFetch } from "../../../lib/api";

export async function POST(req: Request) {
  const body = await req.text();
  const res = await apiFetch("/device-links", { method: "POST", body }, req);
  return new NextResponse(res.body, {
    status: res.status,
    headers: { "content-type": res.headers.get("content-type") ?? "application/json" },
  });
}
