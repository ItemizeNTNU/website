<script context="module">
	import api from '../../utils/api';
	export async function preload(page, session) {
		if (!session.user) this.redirect(302, '/login');
		const { code } = page.params;
		let res = (await api.postCheckin(code, { fetch: this.fetch })).json;
		return { res, code };
	}
</script>

<script>
	export let res;
	export let code;
	console.log(res);
</script>

<svelte:head>
	<title>Innsjekk QR - {code}</title>
</svelte:head>

<main>
	<h1>:)</h1>
	<h3>{res.message}</h3>
</main>
