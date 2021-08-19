<script context="module">
	import api, { fetchResource } from '../utils/api';

	export async function preload(page, session) {
		if (!session.user) {
			this.redirect(302, '/login');
		}
		const dbUser = await api.getUser(session.user.id, { fetch: this.fetch });
		return { user: dbUser.json };
	}
</script>

<script>
	import FaSync from 'svelte-icons/fa/FaSync.svelte';
	import FaTrashAlt from 'svelte-icons/fa/FaTrashAlt.svelte';
	import FaDiscord from 'svelte-icons/fa/FaDiscord.svelte';
	import Button from '../components/Button.svelte';

	export let user;
	let error;

	const refresh = async () => {
		console.log('refreshing user...');
		user = (await api.getUser(user.id)).json;
	};

	const discordRefresh = async () => {
		error = '';
		const resp = await fetchResource('/api/discord/refresh', { method: 'POST', errorText: 'Unable to refresh Discord data: ERROR' });
		if (resp.error) {
			error = resp.error;
		}
		await refresh();
	};
	const discordDelete = async () => {
		error = '';
		const resp = await fetchResource('/api/discord', { method: 'DELETE', errorText: 'Unable to unlink Discord profile: ERROR' });
		if (resp.error) {
			error = resp.error;
		}
		await refresh();
	};
</script>

<svelte:head>
	<title>{user.name}</title>
</svelte:head>

<main>
	{#if error}
		<p class="error">{error}</p>
	{/if}
	<div class="grid">
		<img src={user.imageUrl} alt="Profilbilde" />
		<div class="info">
			<h2>{user.name}</h2>
			<p><b>Epost:</b> <code>{user.email}</code></p>
			<p><b>Fullt Navn:</b> {user.fullName}</p>

			{#if user.discord}
				<p>
					<b>Discord:</b> <code class:error={!user.self.isDiscordMember}>{user.discord}</code><Button submit={discordRefresh} icon={FaSync} /><Button
						submit={discordDelete}
						icon={FaTrashAlt} />
					{#if !user.self.isDiscordMember}
						<br />
						Du har ikke blitt med i Discord-serveren enda. Trykk <a target="_blank" rel="noopener noreferrer" href="https://discord.gg/Et9cCKnyg9">her</a> for å bli med. Trykk så
						på <code>Sync</code> knappen ovenfor for å oppdatere og få tilgang inne på serveren.
					{/if}
				</p>
			{:else}
				<p>
					<b>Discord:</b> <i>Mangler</i>
					<Button href="/api/discord/link" icon={FaDiscord} /> <br /> Du kan bli med på Discord serveren vår
					<a target="_blank" rel="noopener noreferrer" href="https://discord.gg/Et9cCKnyg9">her</a>.
					<br />
					Det krever derimot at du kobler sammen din brukerprofil her med din Discord profil for å få lese tilgang inne på serveren.
				</p>
			{/if}
		</div>
	</div>
</main>

<style>
	.grid {
		display: grid;
		grid-template-columns: 3fr 7fr;
		gap: 2em;
	}
	img {
		height: auto;
		border-radius: 50%;
		border: 0.25em solid #888;
	}
</style>
