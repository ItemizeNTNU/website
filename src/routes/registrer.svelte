<script context="module">
	export async function preload(page, session) {
		if (session.user) {
			this.redirect(302, "/profil");
		}
	}
</script>

<script>
	import { onMount } from "svelte";
	import { fade } from "svelte/transition";

	const html = "<" + "!-- Hysj. Registreringsskjemaet er blitt flyttet hit: https://itemize.no/den-ekte-registreringssiden -->"; // issue https://github.com/sveltejs/prettier-plugin-svelte/issues/244

	let showHelp = true;
	onMount(() => {
		setTimeout(() => (showHelp = true), 0 * 1000);
	});
	let expandHelp = false;
</script>

<svelte:head>
	<title>Bli medlem!</title>
</svelte:head>

<main>
	<h1>Registrer deg og bli medlem!</h1>
	<p>Ønsker du å bli medlem av Itemize NTNU? Itemize NTNU er åpen for studenter og ansatte ved NTNU Trondheim, samt tidligere alumni.</p>
	<p>
		Som medlem har du mulighet for å bli med på Discord serveren vår, hvor all intern kommunikasjon foregår, samt mulighet for å bli med på prosjekter eller stemme ved
		generalforsamling.
	</p>
	<br />
	<p>
		Hm... Det ser ut som det skulle ha vært et registreringsskjema her. Mulig noen har flyttet det? Kanskje det finnes noen spor gjemt i kildekoden på siden her? Mulig du kan finne
		noe med å inspisere litt her på siden?
	</p>

	{#if showHelp}
		<div class="help" transition:fade on:click={() => (expandHelp = !expandHelp)} title="Help?">
			{#if !expandHelp}
				<div class="icon">?</div>
			{:else}
				<div class="text">
					De fleste nettlesere støtter å inspisere kildekoden til nettsider ved å høyre-klikke på siden og velg <code>Inspiser Element</code> eller lignende. Sjekk gjerne ut
					<a target="_blank" rel="noopener noreferrer" href="https://ddg.co?q=how+to+inspect+element+in+BROWSER">her</a> for hjelp med din spesifike nettleser.
					<br />
					Hvis du er på mobil kan det være greit å prøve igjen når en har en PC tilgjengelig.
				</div>
			{/if}
		</div>
	{/if}

	{@html html}
</main>

<style>
	.help {
		position: fixed;
		padding: 0.25em;
		min-width: 2em;
		min-height: 2em;
		bottom: 0;
		right: 0;
		margin: 2em;
		background: #555;
		border: 2px solid #888;
		cursor: pointer;
		font-size: 0.9em;
		max-width: min(40ch, 50vw);
	}
	.icon {
		text-align: center;
	}
</style>
