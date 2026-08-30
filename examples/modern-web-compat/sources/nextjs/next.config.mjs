/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "export",
  trailingSlash: true,
  generateBuildId: async () => "growse-v0.17.0-nextjs",
};

export default nextConfig;
