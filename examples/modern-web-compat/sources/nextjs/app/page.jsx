"use client";

import Link from "next/link";
import { useState } from "react";

export default function Page() {
  const [count, setCount] = useState(0);
  return (
    <main data-framework="nextjs" data-ssr-token="next-ssr-root-v1">
      <p>SSR rendered</p>
      <button type="button" onClick={() => setCount((value) => value + 1)}>Increment</button>
      <output>{count}</output>
      <Link href="/next/about">About</Link>
      <picture>
        <source type="image/avif" srcSet="/assets/unsupported.avif" />
        <img src="/assets/pixel.png" alt="local fixture image" width="32" height="32" />
      </picture>
      <svg width="80" height="48" viewBox="0 0 80 48" aria-label="local fixture SVG">
        <rect width="80" height="48" rx="8" fill="#2563eb" />
      </svg>
    </main>
  );
}
