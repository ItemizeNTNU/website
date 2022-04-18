<script context="module">
	import api from '../utils/api';
	export async function preload(page, session) {
		if (!session.user) this.redirect(302, '/login');
		if (!session.user.roles.includes('Styret')) this.redirect(302, '/arrangementer');
		let event = (await api.getCheckin('ece94523-2b70-43da-8c9d-bf4382d86f6a', { fetch: this.fetch })).json;
		return { event };
	}
</script>

<script>
	import { user } from '../utils/stores';
	import { smartFormat } from '../utils/time';
	import { slide } from 'svelte/transition';
	import ToggleIcon from '../components/ToggleIcon.svelte';
	import FaAngleDown from 'svelte-icons/fa/FaAngleDown.svelte';
	import FaAngleUp from 'svelte-icons/fa/FaAngleUp.svelte';
	let showNew = false;
	export let event;
	console.log(event);
	console.log(event.check_in.attendances);
</script>

<svelte:head>
	<title>Innsjekk</title>
</svelte:head>

<main>
	<h1>Innsjekk</h1>
	<p>
		{event.name} - {smartFormat(event.date)}
	</p>

	<div class="qr" class:old={new Date(event.end) < new Date()}>
		<h3>TODO: Add QR-code here :)</h3>
		<h3>{event.check_in.code}</h3>
	</div>

	{#if $user?.roles?.includes('Styret')}
		<div class="center">
			<ToggleIcon bind:value={showNew} iconOn={FaAngleUp} iconOff={FaAngleDown} />
		</div>
		{#if showNew}
			<div class="attendances" transition:slide>
				<table>
					<thead>
						<tr>
							<th>Navn</th>
							<th>Profil id</th>
							<th>Innsjekk dato</th>
						</tr>
					</thead>
					<tbody>
						{#each event.check_in.attendances ?? [] as checkin}
							<tr>
								<td>{checkin.name}</td>
								<td>{checkin.user_id}</td>
								<td>{smartFormat(checkin.registered)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</main>

<style>
	.qr {
		background: var(--green-1);
		display: flex;
		justify-content: center;
		align-items: center;
		flex-direction: column;
		padding: 2em;
		margin-top: 2em;
	}
	.attendances {
		padding: 0;
	}
	.center {
		min-height: 2em;
		position: relative;
		display: flex;
		flex-direction: row;
		justify-content: center;
	}
	table {
		border-collapse: collapse;
		width: 100%;
	}
	td,
	th {
		border: 1px solid var(--background);
		text-align: left;
		padding: 0.5rem;
	}
	th {
		background: #1e1e1e;
		font-weight: 700;
	}
	tr:nth-child(even) {
		background: #242424;
	}
	tr {
		background: #2b2b2b;
	}
	@media (max-width: 700px) {
		tr > th:nth-of-type(2),
		tr > td:nth-of-type(2) {
			display: none;
		}
	}
</style>
