<script>
  let filter = "all";
  const reports = [
    { id: "CVE-2026-71557", severity: "medium", title: "参照名によるパストラバーサル" },
    { id: "CVE-2026-09086", severity: "high", title: "URI検証バイパスによるXSS" }
  ];
</script>

<svelte:head><title>Growse Real-site fixture</title></svelte:head>
<main class="mx-auto max-w-5xl p-6 font-sans text-slate-900">
  <h1 class="text-2xl font-bold">🔎 脆弱性報告</h1>
  <nav class="my-6 flex gap-3 rounded-card bg-white p-card shadow-sm">
    <button on:click={() => filter = "all"}>すべて</button>
    <button on:click={() => filter = "high"}>HIGHのみ</button>
  </nav>
  <section class="grid gap-4 md:grid-cols-2">
    {#each reports.filter((report) => filter === "all" || report.severity === filter) as report}
      <article class="rounded-card border border-slate-200 bg-white p-card shadow-sm">
        <img src="/assets/pixel.png" alt="" class="h-16 w-full rounded-lg object-cover" />
        <h2 class="mt-3 font-mono font-bold">{report.id}</h2>
        <p>{report.title}</p>
      </article>
    {/each}
  </section>
</main>
