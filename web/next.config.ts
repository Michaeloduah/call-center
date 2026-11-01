import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Every page is per-user and rendered per request from the Go API, so Cache Components'
  // static-shell prerendering offers no benefit here; standard dynamic rendering is used instead.
  cacheComponents: false,
  poweredByHeader: false,
  // Keep the dev badge clear of the sidebar's "Sign out" button.
  devIndicators: { position: "bottom-right" },
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
