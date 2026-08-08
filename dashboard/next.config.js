/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Emit a self-contained server bundle so the container image does not need
  // node_modules or the pnpm store at runtime.
  output: "standalone",
};

module.exports = nextConfig;
