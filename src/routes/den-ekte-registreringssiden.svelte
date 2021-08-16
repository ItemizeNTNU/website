<script context="module">
	export async function preload(page, session) {
		if (session.user) {
			this.redirect(302, '/profil');
		}
	}
</script>

<script>
	import { goto } from '@sapper/app';
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
	let error = {
		email: ''
	};
	let resp;
	let register = async () => {
		resp = null;
		resp = await api.registerUser(user);
		if (!resp.error) {
			await goto('/registrert');
		}
	};
</script>

<svelte:head>
	<title>Bli medlem!</title>
</svelte:head>

<main>
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
	<button class="full" on:click={register}>Registrer</button>
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
