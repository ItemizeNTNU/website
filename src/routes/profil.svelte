<script context="module">
	export async function preload(page, session) {
		if (!session.user) {
			this.redirect(302, "/login");
		}
	}
</script>

<script>
	import { user } from "../utils/stores";
	let discordUsername = $user.discord;
</script>

<svelte:head>
	<title>{$user.name}</title>
</svelte:head>

<main>
	<img src={$user.imageUrl} alt="Profilbilde" />
	<div class="info">
		<h2>Hei {$user.name}</h2>
		<p>Epost: <code>{$user.email}</code></p>
		<p>Fullt Navn: {$user.fullName}</p>
		<p>
			<b>Discord:</b>
			<br />
			Bli med på discord serveren vår <a target="_blank" rel="noopener noreferrer" href="https://discord.gg/Et9cCKnyg9">her</a>. <br />Send så en e-post til
			<a href="mailto:hack@itemize.no">hack@itemize.no</a> med ditt Discord brukernavn for å bli lagt til. Bruk samme e-post som ovenfor.
		</p>
	</div>
</main>

<style>
	main {
		display: grid;
		grid-template-columns: 3fr 7fr;
		gap: 2em;
	}
	code {
		grid-column: 1 / 3;
	}
	img {
		height: auto;
	}
</style>
