<script context="module">
	import api, { fetchResource } from '../utils/api';


	export async function preload(page, session) {
		if (!session.user) {
			this.redirect(302, '/login');
		}else if(!session.user?.roles.includes('Styret')){
			this.redirect(401, '/')
		}
		
		const dbUser = await api.getUser(session.user.id, { fetch: this.fetch });
		return { user: dbUser.json };
	}
</script>

<script>
	export let user;
	let error;
	console.log("ikke sess")
	console.log(user)
</script>

<svelte:head>
	<title>Admin panel</title>
</svelte:head>

<main>

</main>
