<script>
  let revision = 0;
  let name = "SSR";
  let result = "not submitted";

  function enhance(event) {
    event.preventDefault();
    result = `enhanced:${name}`;
  }
</script>

<svelte:head><title>SvelteKit SSR compatibility fixture</title></svelte:head>
<main data-ssr-token="svelte-ssr-root-v1">
  <p>SSR rendered</p>
  <button type="button" on:click={() => revision += 1}>Update</button>
  <output>reactive:{revision}</output>
  <form method="post" action="/svelte/action" on:submit={enhance}>
    <input name="name" bind:value={name}>
    <button type="submit">Save</button>
  </form>
  <output>{result}</output>
  <a href="/svelte/about">About</a>
</main>
