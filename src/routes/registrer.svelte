<script context="module">
	export async function preload(page, session) {
		if (session.user) {
			this.redirect(302, '/profil');
		}
	}
</script>

<script>
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';

	const html = '<' + '!-- Hysj. Registreringsskjemaet er blitt flyttet hit: https://itemize.no/den-ekte-registreringssiden -->'; // issue https://github.com/sveltejs/prettier-plugin-svelte/issues/244

	let showHelp = true;
	onMount(() => setTimeout(() => (showHelp = true), 0 * 1000));
	let expandHelp = false;
	const toggleHelp = (e) => {
		if (e.target.tagName === 'BUTTON') expandHelp = false;
		else expandHelp = true;
	};
</script>

<svelte:head>
	<title>Bli medlem!</title>
</svelte:head>

<main>
	<h1>Registrer deg og bli medlem!</h1>
	<p>Ønsker du å bli medlem av Itemize NTNU? Itemize NTNU er åpen for studenter og ansatte ved NTNU Trondheim, samt tidligere alumni.</p>
	<p>
		Som medlem har du mulighet for å bli med på Discord-serveren vår, hvor all intern kommunikasjon foregår, samt mulighet for å bli med på prosjekter eller stemme ved
		generalforsamling.
	</p>
	<br />
	<p>
		Hm... Det ser ut som det skulle ha vært et registreringsskjema her. Mulig noen har flyttet det? Kanskje det finnes noen spor gjemt i kildekoden på siden her? Mulig du kan finne
		noe med å inspisere litt her på siden?
	</p>

	{#if showHelp}
		<div class="help" class:expanded={expandHelp} transition:fade on:click={toggleHelp} title="Help?">
			<div class="icon">?</div>

			<div class="text">
				De fleste nettlesere støtter å inspisere kildekoden til nettsider ved å høyre-klikke på siden og velg <code>Inspiser Element</code> eller lignende. Sjekk gjerne ut
				<a target="_blank" rel="noopener noreferrer" href="https://ddg.co?q=how+to+inspect+element+in+BROWSER">her</a> for hjelp med din spesifike nettleser.
				<br />
				Hvis du er på mobil kan det være greit å prøve igjen når en har en PC tilgjengelig.
				<button on:click={toggleHelp}>Lukk hintet</button>
			</div>
		</div>
	{/if}

	{@html html}
</main>

<style>
	main {
		position: relative;
		padding-bottom: 70px;
	}
	.help {
		position: absolute;
		padding: 0.25em;
		min-width: 4em;
		min-height: 4em;
		bottom: -10px;
		right: -10px;
		margin: 2em;
		background: #555;
		border: 2px solid #888;
		cursor: pointer;
		font-size: 0.95em;
		max-width: min(40ch, 50vw);
		display: flex;
		border-radius: 50%;
		align-items: center;
		justify-content: center;
	}
	.help div {
		transition: transform 0.5s ease;
	}
	.help .text {
		width: 400px;
		position: absolute;
		bottom: 0;
		opacity: 0;
		pointer-events: none;
		/* transform: scale(0.4); */
	}
	.help.expanded {
		border-radius: 0;
		cursor: auto;
	}
	.help.expanded .icon {
		display: none;
	}
	.help.expanded .text {
		position: relative;
		opacity: 1;
		pointer-events: all;
		/* transform: scale(1); */
		left: 0;
	}
	.icon {
		text-align: center;
		font-size: 2.5em;
		line-height: 1;
	}
	.text {
		padding: 10px;
	}
	button {
		margin: 10px auto 0 auto;
		width: 100%;
	}
</style>
