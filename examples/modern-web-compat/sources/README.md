# Framework fixture source

This directory preserves the authored inputs represented by the checked-in HTTP artifacts in `../fixtures`.
The v0.17 Next.js and SvelteKit directories are buildable upstream applications. Their complete static build output is
checked in below each fixture's `upstream-export` directory and covered by `upstream.sha256`. The small `app.mjs`
files remain deterministic compatibility bootstraps used to exercise Growse's module loader without teaching the Go
engine framework-specific behavior. The `real-site` CSS is the checked-in output of Tailwind CLI 4.1.12, paired with
SvelteKit SSR source and a deterministic hydration contract. It includes Japanese text, responsive grid utilities,
duplicate images, custom theme tokens, and transform / opacity animation.

The exact upstream versions, offline build command, licenses, and artifact digests are recorded in `../fixture-manifest.json`.
Framework output is passed through `normalize-framework-build.mjs`, which replaces security-sensitive invisible
Unicode characters with equivalent JavaScript Unicode escapes and removes trailing whitespace before checksums are recorded. CI verifies the checked-in
bytes and never runs pnpm, accesses npm, or requires public DNS.
