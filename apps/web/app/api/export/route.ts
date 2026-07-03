import { apiFetch } from "../../../lib/api";

export async function GET(req: Request) {
  const res = await apiFetch("/export", undefined, req);
  if (!res.ok) {
    return new Response(res.body, {
      status: res.status,
      headers: { "content-type": "application/json" },
    });
  }
  const stamp = new Date().toISOString().slice(0, 10).replaceAll("-", "");
  return new Response(res.body, {
    status: res.status,
    headers: {
      "content-type": "application/json",
      "content-disposition": `attachment; filename="openmind-export-${stamp}.json"`,
    },
  });
}
