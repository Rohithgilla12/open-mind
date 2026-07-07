import Link from "next/link";
import { tokens } from "@openmind/ui";
import { Shell } from "../../../components/Shell";
import { DevicesKeys } from "../../../components/DevicesKeys";

const { color } = tokens;

export default function DevicesPage() {
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
        <h1
          className="serif"
          style={{ fontSize: 27, fontWeight: 600, letterSpacing: "-.02em", margin: "10px 0 4px", color: color.ink }}
        >
          Devices &amp; keys
        </h1>
        <p className="meta" style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt }}>
          Link new devices and manage the API keys that can act on your behalf.
        </p>
      </div>
      <div style={{ padding: "26px 28px 40px" }}>
        <DevicesKeys />
      </div>
    </Shell>
  );
}
