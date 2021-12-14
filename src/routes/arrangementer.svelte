<script context="module">
	import api from '../utils/api';
	export async function preload() {
		let events = (await api.getEvents(false, { fetch: this.fetch })).json;
		return { events };
	}
</script>

<script>
	import ToggleIcon from '../components/ToggleIcon.svelte';
	import { user } from '../utils/stores';
	import { slide } from 'svelte/transition';
	import FaCog from 'svelte-icons/fa/FaCog.svelte';
	import FaEyeSlash from 'svelte-icons/fa/FaEyeSlash.svelte';
	import FaCalendarCheck from 'svelte-icons/fa/FaCalendarCheck.svelte';
	import FaTrash from 'svelte-icons/fa/FaTrash.svelte';
	import FaExternalLinkAlt from 'svelte-icons/fa/FaExternalLinkAlt.svelte';
	import Button from '../components/Button.svelte';
	import { DateTime } from 'luxon';
	import TimePicker from '../components/TimePicker.svelte';
	import Icon from '../components/Icon.svelte';
	export let events;
	let error;
	let showNew = false;
	let showOld = false;
	const newEventDefault = {
		name: '',
		location: {
			name: '',
			url: ''
		},
		register_url: '',
		date: '',
		duration: 5.0,
		ctf: {
			name: '',
			url: ''
		},
		info: '',
		hidden: false
	};
	let newEvent;
	const resetNewEvent = (data = newEventDefault, show) => {
		newEvent = JSON.parse(JSON.stringify(data));
		newEvent.date = data.date ? new Date(data.date) : new Date();
		if (show) showNew = true;
	};
	resetNewEvent();

	const refresh = async () => {
		events = (await api.getEvents(showOld)).json;
	};

	const smartFormat = (date) => {
		if (['number', 'string'].includes(typeof date)) {
			date = new Date(date);
		}
		if (date instanceof Date) {
			date = DateTime.fromJSDate(date);
		}

		date = date.setZone('Europe/Oslo').setLocale('no');
		const now = DateTime.now().setZone('Europe/Oslo');
		let format = "'I dag kl' HH:mm";
		if (date.month != now.month || date.day != now.day) {
			format = "EEEE d. MMMM 'kl' HH:mm";
		}
		if (date.year != now.year) {
			format = "EEEE d. MMMM y 'kl' HH:mm";
		}
		return date.toFormat(format);
	};

	const postEvent = async () => {
		error = '';
		const res = await api.postEvent(newEvent);
		if (res.error) {
			error = res.error;
			return;
		}
		resetNewEvent();
		refresh();
	};

	const deleteEvent = async () => {
		if (confirm('Er du sikker på at du vil slette dette arrangementet?\nDette kan ikke angres.')) {
			error = '';
			const res = await api.deleteEvent(newEvent._id);
			if (res.error) {
				error = res.error;
				return;
			}
			resetNewEvent();
			refresh();
		}
	};
</script>

<svelte:head>
	<title>Arrangementer</title>
</svelte:head>

<main>
	<h1 class="title">
		Arrangementer
		<span class="right small">
			<a href="/api/events/ical" title="iCal feed"><Icon><FaExternalLinkAlt /></Icon></a>
		</span>
	</h1>
	<p>Her finner du oversikt over våre fremtidige arrangementer.</p>
	{#if $user?.roles?.includes('Styret')}
		<div class="center">
			<ToggleIcon bind:value={showNew} />
			<span class="right">
				<ToggleIcon bind:value={showOld} onToggle={refresh} iconOff={FaCalendarCheck} colorOff="#888" rotate="0" />
			</span>
		</div>
		{#if error}
			<p class="error">{error}</p>
		{/if}
		{#if showNew}
			<form transition:slide>
				<label><span class="col">Navn:</span> <input type="text" bind:value={newEvent.name} /></label>
				<label><span class="col">Hvor:</span> <input type="text" bind:value={newEvent.location.name} /></label>
				<label><span class="col">Hvor link:</span> <input type="text" bind:value={newEvent.location.url} /> (e.g. mazemap link)</label>
				<label><span class="col">Registrer link:</span> <input type="text" bind:value={newEvent.register_url} /> (link for registrering ved fysisk oppmøte)</label>
				<span><span class="col">Når: </span> <TimePicker bind:date={newEvent.date} /> {smartFormat(newEvent.date)}</span>
				<label
					><span class="col">Varighet:</span> <input type="number" bind:value={newEvent.duration} min="0" /> (lengde i timer, slutter {smartFormat(
						DateTime.fromJSDate(newEvent.date).plus({ hours: newEvent.duration })
					)})</label
				>
				<label><span class="col">CTF Navn:</span> <input type="text" bind:value={newEvent.ctf.name} /></label>
				<label><span class="col">CTF Link:</span> <input type="text" bind:value={newEvent.ctf.url} /></label>
				<label><span class="col">Info:</span> <textarea rows="3" bind:value={newEvent.info} /></label>
				<label><span class="col">Skjult:</span> <input type="checkbox" bind:value={newEvent.hidden} bind:checked={newEvent.hidden} /></label>
				<span class="col" /> <button on:click|preventDefault={postEvent}>{newEvent._id ? 'Oppdater' : 'Legg til'}</button>
				<Button icon={FaTrash} title="Delete Event" disabled={!newEvent._id} submit={deleteEvent} />
			</form>
		{/if}
	{:else}
		<div class="center right">
			<ToggleIcon bind:value={showOld} onToggle={refresh} iconOff={FaCalendarCheck} colorOff="#888" rotate="0" />
		</div>
	{/if}
	<ul>
		{#each events as event}
			<li class="card" class:old={new Date(event.end) < new Date()}>
				{#if $user?.roles?.includes('Styret')}
					<div class="buttons">
						{#if event.hidden}
							<Button disabled icon={FaEyeSlash} />
						{/if}
						<Button submit={() => resetNewEvent(event, true)} icon={FaCog} />
					</div>
				{/if}
				<h3>{event.name} <span class="done">{new Date(event.end) < new Date() ? '[Ferdig]' : ''}</span></h3>
				<table>
					<tbody>
						<tr>
							<td>Hvor:</td>
							<td>
								{#if event.location.url}
									<a href={event.location.url} target="_blank" rel="noopener noreferrer">{event.location.name}</a>
								{:else}{event.location.name}{/if}
							</td>
						</tr>
						<tr>
							<td>Når:</td>
							<td>{smartFormat(event.date)}</td>
						</tr>
						{#if event.register_url}
							<tr>
								<td>Registrering:</td>
								<td><a href={event.register_url} target="_blank" rel="noopener noreferrer">her</a></td>
							</tr>
						{/if}
						{#if event.ctf && event.ctf.name}
							<td>CTF:</td>
							<td>
								{#if event.ctf.url}
									<a href={event.ctf.url} target="_blank" rel="noopener noreferrer">
										{event.ctf.name}
									</a>
								{:else}{event.ctf.name}{/if}
							</td>
						{/if}
						<tr>
							<td>Info:</td>
							<td>
								{@html event.info}
							</td>
						</tr>
					</tbody>
				</table>
			</li>
		{:else}
			<li>
				<p>Det ser ikke ut som vi har noen planlagte arrangementer annonsert enda. Kom gjerne tilbake igjen senere 🙃</p>
			</li>
		{/each}
	</ul>
</main>

<style>
	li {
		list-style-type: none;
		background: rgba(255, 255, 255, 0.1);
		padding: 0.5em;
		margin-bottom: 1em;
		border: 2px solid rgba(0, 0, 0, 0);
		transition: border 0.2s;
	}
	.old {
		--A: #ffffff06;
		--B: #ffffff10;
		--width: 3em;
		background: repeating-linear-gradient(-45deg, var(--A), var(--A) var(--width), var(--B) var(--width), var(--B) calc(var(--width) * 2));
	}
	.done {
		color: var(--gray-2);
	}

	li:hover {
		border: 2px solid rgba(255, 255, 255, 0.05);
		transition: border 0s;
	}

	li p {
		text-align: center;
	}

	ul {
		padding: 0;
	}

	td {
		padding: 0 0.5em;
		text-align: left;
		vertical-align: top;
	}
	.title {
		position: relative;
	}
	.small {
		font-size: 0.7em;
		top: 10px;
	}
	.center {
		min-height: 2em;
		position: relative;
		display: flex;
		flex-direction: row;
		justify-content: center;
	}
	:is(.title, .center) .right {
		position: absolute;
		right: 0;
	}
	.center.right {
		justify-content: end;
	}
	label {
		display: block;
		padding: 0.25em 0;
	}
	span.col {
		width: 8em;
		display: inline-block;
		text-align: end;
		vertical-align: top;
	}

	input:not([type='checkbox']),
	textarea,
	button {
		width: 17.3em;
	}

	.card {
		position: relative;
	}
	.buttons {
		position: absolute;
		right: 0;
		top: 0;
		display: flex;
		flex-direction: row;
		padding: 0.5em;
	}
</style>
