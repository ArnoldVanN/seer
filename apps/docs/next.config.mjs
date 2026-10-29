import { createMDX } from 'fumadocs-mdx/next';

const withMDX = createMDX();

/** @type {import('next').NextConfig} */
const config = {
  // Static export, served by nginx (see Dockerfile).
  output: 'export',
  reactStrictMode: true,
};

export default withMDX(config);
