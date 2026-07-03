import { cookies } from "next/headers";
import { NextResponse } from "next/server";

const API_URL = process.env.API_URL ?? "http://localhost:8080";

export async function POST(req: Request) {
  const { token } = (await req.json()) as { token?: string };
  if (!token) return NextResponse.json({ error: "token required" }, { status: 400 });
  let probe: Response;
  try {
    probe = await fetch(`${API_URL}/items?limit=1`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    });
  } catch {
    return NextResponse.json({ error: "api unreachable" }, { status: 502 });
  }
  if (!probe.ok) {
    if (probe.status === 401)
      return NextResponse.json({ error: "invalid token" }, { status: 401 });
    if (probe.status === 429)
      return NextResponse.json({ error: "rate limited, retry shortly" }, { status: 429 });
    return NextResponse.json({ error: "api error" }, { status: 502 });
  }
  (await cookies()).set("om_token", token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 90,
  });
  return NextResponse.json({ ok: true });
}

export async function DELETE() {
  (await cookies()).delete("om_token");
  return NextResponse.json({ ok: true });
}
