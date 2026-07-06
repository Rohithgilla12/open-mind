import { cookies } from "next/headers";

const API_URL = process.env.API_URL ?? "http://localhost:8080";

// Proxy the MCP Streamable-HTTP endpoint through to the API binary. The API is
// never publicly exposed — the web container is the single ingress — so /mcp
// must be forwarded the same way /api/* is. MCP clients send their token as an
// `Authorization: Bearer` header; a logged-in browser's om_token cookie is used
// as a fallback. The response (which may be a text/event-stream) is streamed
// through untouched, and the MCP session header is forwarded both ways.
async function proxy(
  req: Request,
  ctx: { params: Promise<{ path?: string[] }> },
): Promise<Response> {
  const { path } = await ctx.params;
  const suffix = path && path.length > 0 ? `/${path.join("/")}` : "";
  const target = `${API_URL}/mcp${suffix}`;

  const headers = new Headers();
  for (const h of [
    "accept",
    "content-type",
    "mcp-session-id",
    "mcp-protocol-version",
    "last-event-id",
  ]) {
    const v = req.headers.get(h);
    if (v) headers.set(h, v);
  }

  const incoming = req.headers.get("authorization");
  if (incoming?.startsWith("Bearer ")) {
    headers.set("authorization", incoming);
  } else {
    const token = (await cookies()).get("om_token")?.value;
    if (token) headers.set("authorization", `Bearer ${token}`);
  }

  const body =
    req.method === "GET" || req.method === "HEAD"
      ? undefined
      : await req.arrayBuffer();
  const upstream = await fetch(target, {
    method: req.method,
    headers,
    body,
    cache: "no-store",
  });

  const respHeaders = new Headers();
  for (const h of ["content-type", "mcp-session-id", "cache-control"]) {
    const v = upstream.headers.get(h);
    if (v) respHeaders.set(h, v);
  }
  return new Response(upstream.body, {
    status: upstream.status,
    headers: respHeaders,
  });
}

export const GET = proxy;
export const POST = proxy;
export const DELETE = proxy;
export const dynamic = "force-dynamic";
