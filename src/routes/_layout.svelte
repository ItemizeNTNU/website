<script context="module">
	export async function preload() {
		const res = await this.fetch("/api/user");
		const json = await res.json();
		return { userData: json.user };
	}
</script>

<script>
	import Nav from "../components/Nav.svelte";
	import { user } from "../utils/stores";

	export let segment;
	export let userData;
	$user = userData;
</script>

<Nav {segment} />

<slot />
