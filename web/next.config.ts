import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  // Pin the tracing root to this app; a stray lockfile higher up the tree
  // otherwise makes Next guess the wrong workspace root.
  outputFileTracingRoot: __dirname,
};

export default nextConfig;
