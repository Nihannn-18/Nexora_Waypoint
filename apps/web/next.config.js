//@ts-check

/** @type {import('next').NextConfig} */
const nextConfig = {
  // `standalone` emits apps/web/.next/standalone with only the files the server
  // actually needs, so the Docker runtime stage copies a few megabytes instead
  // of the whole workspace's node_modules.
  output: 'standalone',

  // In a monorepo, Next traces dependencies from the app folder by default and
  // misses the hoisted root node_modules and the libs under libs/*. Pointing the
  // trace root at the workspace root is what makes `output: 'standalone'` work
  // here at all.
  outputFileTracingRoot: require('node:path').join(__dirname, '../../'),

  // The browser talks to the Go API directly, so the base URL has to be
  // readable from client components. It is a public URL, not a secret.
  env: {
    NEXT_PUBLIC_API_BASE_URL:
      process.env.NEXT_PUBLIC_API_BASE_URL ?? 'http://localhost:8080/api/v1',
  },

  // See: https://nextjs.org/docs/app/api-reference/config/next-config-js
};

module.exports = nextConfig;
