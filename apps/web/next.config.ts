import type { NextConfig } from "next";
import path from "node:path";

const nextConfig: NextConfig = {
  output: "standalone",
  // Monorepo: trace files from the repo root so workspace packages
  // (@openmind/ui, @openmind/api-client) are bundled into the standalone output.
  outputFileTracingRoot: path.join(__dirname, "../.."),
  transpilePackages: ["@openmind/ui", "@openmind/api-client"],
};

export default nextConfig;
