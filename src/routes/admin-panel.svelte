<script context="module">
	import api from '../utils/api';


	export async function preload(page, session) {
		if (!session.user) {
			this.redirect(302, '/login');
		}else if(!session.user?.roles.includes('Styret')){
			this.redirect(401, '/')
		}
		const query = 'queryString=*'
		const dbUser = await api.searchUsers(query,{ fetch: this.fetch });
		return { users: dbUser.json };
	}
</script>

<script>
	export let users;
	console.log("ikke sess")
	console.log(users)
</script>

<svelte:head>
	<title>Admin panel</title>
</svelte:head>

<main>

</main>
