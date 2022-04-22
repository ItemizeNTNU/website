<script context="module">
	import api from '../../utils/api';
	export async function preload(page, session) {
		if (!session.user) this.redirect(302, '/login');
		if (!session.user.roles.includes('Styret')) this.redirect(302, '/arrangementer');
		const { code } = page.params;
		let event = (await api.getCheckin(code, { fetch: this.fetch })).json;
		return { event };
	}
</script>

<script>
	import { onMount } from 'svelte';
	import { smartFormat } from '../../utils/time';
	import { slide } from 'svelte/transition';
	import ToggleIcon from '../../components/ToggleIcon.svelte';
	import FaAngleDown from 'svelte-icons/fa/FaAngleDown.svelte';
	import FaAngleUp from 'svelte-icons/fa/FaAngleUp.svelte';
	import QRCode from 'qrcode';
	let showNew = false;
	export let event;
	let canvas;
	onMount(() => {
		const opts = {
			width: 300
		};
		if (event.check_in.code)
			QRCode.toCanvas(canvas, `${window.location.origin}/innsjekk-qr/${event.check_in.code}`, opts, function (error) {
				if (error) console.error(error);
			});
	});
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
		<canvas bind:this={canvas} />
		<br />
		<h3><a href={`/innsjekk-qr/${event.check_in.code}`}>{event.check_in.code}</a></h3>
	</div>

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
</main>

<style>
	.qr {
		display: flex;
		justify-content: center;
		align-items: center;
		flex-direction: column;
		padding: 2em 0;
		margin-top: 2em;
	}
	canvas {
		max-width: 80vw;
		max-height: 80vw;
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
