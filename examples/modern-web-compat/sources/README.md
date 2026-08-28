# Framework fixture source

This directory preserves the smallest authored inputs represented by the checked-in HTTP artifacts in `../fixtures`.
The artifacts are reduced compatibility contracts: they preserve the observable SSR, bootstrap, hydration, interaction,
navigation, and CSS behavior emitted by the pinned tools without vendoring a framework runtime or requiring npm in CI.

The exact upstream versions, offline build commands, licenses, and artifact digests are recorded in `../fixture-manifest.json`.
