import type { NextConfig } from "next";

// Browser talks same-origin (/api/*); Next rewrites to the API so session cookies
// are first-party to the web UI (avoids cross-port cookie loss on refresh).
const apiInternal = (process.env.API_INTERNAL_URL || "http://127.0.0.1:8080").replace(/\/$/, "");

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiInternal}/api/:path*` },
      { source: "/healthz", destination: `${apiInternal}/healthz` },
    ];
  },
};

export default nextConfig;
