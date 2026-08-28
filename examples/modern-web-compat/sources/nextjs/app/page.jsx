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
    </main>
  );
}
