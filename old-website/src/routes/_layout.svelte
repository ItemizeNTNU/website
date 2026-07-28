<script context="module">
	export async function preload(page, session) {
		return { userData: session?.user };
	}
</script>

<script>
	import Nav from '../components/Nav.svelte';
	import Footer from '../components/Footer.svelte';
	import { user } from '../utils/stores';
	import { ToastContainer, FlatToast } from 'svelte-toasts';
	import PromptContainer from '../components/PromptContainer.svelte';

	export let segment;
	export let userData;
	$user = userData;
</script>

<Nav {segment} />

<slot />

<ToastContainer placement="bottom-right" duration={10000} showProgress let:data>
	<FlatToast {data} />
</ToastContainer>

<PromptContainer />

<Footer />
