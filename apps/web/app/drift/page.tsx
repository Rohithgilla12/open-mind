import { getDrift } from "../../lib/drift";
import { DriftFlow } from "./DriftFlow";

// Drift is the one full-screen, immersive experience — deliberately NOT wrapped
// in Shell (no sidebar). It server-fetches the batch once; DriftFlow drives the
// one-at-a-time flow on the client.
export default async function DriftPage() {
  const { items, total } = await getDrift();
  return <DriftFlow items={items} total={total} />;
}
