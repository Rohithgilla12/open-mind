import Link from "next/link";
import { tokens } from "@openmind/ui";
import { Shell } from "../../../components/Shell";
import { LensForm } from "../../../components/LensForm";

const { color } = tokens;

export default async function NewLensPage({
  searchParams,
}: {
  searchParams: Promise<{ q?: string; color?: string; types?: string; domains?: string }>;
}) {
  const { q, color: colorParam, types, domains } = await searchParams;
  const initialTypes = types ? types.split(",").filter(Boolean) : [];
  const initialDomains = domains?.split(",").filter(Boolean) ?? [];

  return (
    <Shell>
      <div
        style={{
          height: 2,
          background: `linear-gradient(90deg,${color.terracotta},${color.terracotta} 40%,transparent)`,
        }}
      />
      <div style={{ padding: "24px 28px", borderBottom: `1px solid ${color.hairline}`, background: color.header }}>
        <Link
          href="/"
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".02em", color: color.cobalt, textDecoration: "none" }}
        >
          ← The Mind
        </Link>
        <h1 className="serif" style={{ fontSize: 27, fontWeight: 600, letterSpacing: "-.02em", margin: "10px 0 4px", color: color.ink }}>
          New Lens
        </h1>
        <p className="meta" style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt }}>
          A saved query you look at your collection through — new saves appear automatically.
        </p>
      </div>
      <div style={{ padding: "26px 28px 40px" }}>
        <LensForm
          initialQ={q}
          initialColor={colorParam}
          initialTypes={initialTypes}
          initialDomains={initialDomains}
        />
      </div>
    </Shell>
  );
}
