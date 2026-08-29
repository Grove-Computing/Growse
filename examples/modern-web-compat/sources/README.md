# Framework fixture source

This directory preserves the smallest authored inputs represented by the checked-in HTTP artifacts in `../fixtures`.
The v0.15 framework artifacts are reduced compatibility contracts. The v0.16 `real-site` CSS is the checked-in output of
Tailwind CLI 4.1.12, paired with SvelteKit SSR source and a deterministic hydration contract. It includes Japanese text,
responsive grid utilities, duplicate images, custom theme tokens, and transform / opacity animation.

The exact upstream versions, offline build command, licenses, and artifact digests are recorded in `../fixture-manifest.json`.
CI verifies the checked-in bytes and never runs pnpm, accesses npm, or requires public DNS.
