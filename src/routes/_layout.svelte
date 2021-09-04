<script context="module">
	export async function preload(page, session) {
		return { userData: session?.user };
	}
</script>

<script>
	import Nav from '../components/Nav.svelte';
	import Footer from '../components/Footer.svelte';
	import { user } from '../utils/stores';

	export let segment;
	export let userData;
	$user = userData;
</script>

<Nav {segment} />

<slot />

<Footer />
