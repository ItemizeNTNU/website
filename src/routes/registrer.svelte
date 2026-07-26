<script context="module">
	export async function preload(page, session) {
		if (session.user) {
			this.redirect(302, '/profil');
		}
	}
</script>

<script>
	import { goto } from '@sapper/app';
	import Button from '../components/Button.svelte';
	import api from '../utils/api';

	const types = {
		student: 'Student',
		employee: 'Ansatt',
		alumni: 'Alumni'
	};
	let user = {
		fullName: '',
		email: '',
		data: {
			type: Object.keys(types)[0],
			displayName: '',
			study: {
				program: '',
				year: 1
			},
			alumni: {
				joinYear: new Date().getFullYear()
			},
			employee: {
				title: ''
			}
		}
	};
	let expectedFinishYear = new Date().getFullYear();
	// Studieslutt lagres som ISO8601 dato, 1. juni i det oppgitte året.
	const yearToISO = (year) => (year && Number.isInteger(+year) ? new Date(Date.UTC(+year, 5, 1)).toISOString() : '');
	let error = {
		email: ''
	};
	let resp;
	let register = async () => {
		resp = null;
		const payload = { ...user, data: { ...user.data, study: { ...user.data.study, expectedFinishYear: yearToISO(expectedFinishYear) } } };
		resp = await api.registerUser(payload);
		if (!resp.error) {
			await goto('/registrert');
		}
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

	{#if resp?.error}
		<p class="error">{resp.error.replace(/.*failed custom validation because /, '') || 'Ups. Noe gikk galt :/'}</p>
	{/if}
	<div class="form">
		<div class="cell">Fullt Navn:</div>
		<div class="cell"><input type="text" bind:value={user.fullName} /></div>
		<div class="comment">Fullt navn kan kun settes en gang.</div>

		<div class="cell">E-post adresse:</div>
		<div class="cell"><input type="text" bind:value={user.email} /></div>
		{#if error.email}
			<div class="comment error">{error.email}</div>
		{/if}

		<div class="cell">Visningsnavn:</div>
		<div class="cell"><input type="text" bind:value={user.data.displayName} /></div>
		<div class="comment">Ditt navn på nettsiden og i e-poster. Dette kan endres senere.</div>

		<div class="cell">Medlemstype:</div>
		<div class="cell">
			<select bind:value={user.data.type}>
				{#each Object.keys(types) as type}
					<option value={type}>{types[type]}</option>
				{/each}
			</select>
		</div>
		{#if user.data.type == 'student'}
			<div class="comment">Student ved NTNU Trondheim.</div>
		{:else if user.data.type == 'alumni'}
			<div class="comment">Tidligere Itemize medlem.</div>
		{:else if user.data.type == 'employee'}
			<div class="comment">Ansatt ved NTNU.</div>
		{/if}

		{#if user.data.type == 'student' || user.data.type == 'alumni'}
			<div class="cell">Studieprogram:</div>
			<div class="cell"><input type="text" bind:value={user.data.study.program} /></div>
			{#if user.data.type == 'student'}
				<div class="cell">Studieår:</div>
				<div class="cell"><input type="number" min="1" max="100" bind:value={user.data.study.year} /></div>
				<div class="comment">Nåværende progresjonsår i studiet. Vanligvis mellom 1 og 5.</div>
				<div class="cell">Forventet ferdig år:</div>
				<div class="cell"><input type="number" min={new Date().getFullYear()} max={new Date().getFullYear() + 10} bind:value={expectedFinishYear} /></div>
				<div class="comment">Hvilket år regner du med å være ferdig med studiet ditt?</div>
			{:else if user.data.type == 'alumni'}
				<div class="cell">Medlems år:</div>
				<div class="cell"><input type="number" min="2014" max={new Date().getFullYear()} bind:value={user.data.alumni.joinYear} /></div>
				<div class="comment">Hvis du er tideligere alumni, hvilket år ble du først med i Itemize?</div>
			{/if}
		{:else if user.data.type == 'employee'}
			<div class="cell">Title:</div>
			<div class="cell"><input type="text" bind:value={user.data.employee.title} /></div>
			<div class="comment">Fyll ut best beskrivende jobb tittel.</div>
		{/if}
	</div>
	<Button fill big submit={register}>Register</Button>
</main>

<style>
	.form {
		display: grid;
		grid-template-columns: 50% 50%;
		align-items: center;
	}
	.form * {
		margin: 0.4em 0;
	}
	.cell * {
		width: 100%;
	}
	.comment {
		grid-column: 1/3;
		font-size: 0.8em;
		margin: -1em 0 1em;
	}
</style>
