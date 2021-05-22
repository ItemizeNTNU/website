<script context="module">
	export async function preload(page, session) {
		const res = await this.fetch("/api/events");
		const events = await res.json();
		return { events };
	}
</script>

<script>
	import ToggleIcon from "../components/ToggleIcon.svelte";
	import { user } from "../utils/stores";
	import { slide } from "svelte/transition";
	import FaCog from "svelte-icons/fa/FaCog.svelte";
	import FaEyeSlash from "svelte-icons/fa/FaEyeSlash.svelte";
	import Button from "../components/Button.svelte";
	export let events;
	let error;
	let showNew = false;
	const newEventDefault = {
		name: "",
		location: {
			name: "",
			url: "",
		},
		register_url: "",
		date: "",
		ctf: {
			name: "",
			url: "",
		},
		info: "",
		hidden: false,
	};
	let newEvent = JSON.parse(JSON.stringify(newEventDefault));
	const notEmpty = (name, value) => {
		if (!value) {
			throw name + " kan ikke være tomt";
		}
	};
	const notURL = (name, value) => {
		if (value && !value.startsWith("https://")) {
			throw name + " må starte med 'https://'";
		}
	};
	const zfill = (value, digits = 2) => {
		value = String(value);
		while (value.length < digits) value = "0" + value;
		return value;
	};
	const parseDate = (value, strict = true) => {
		let date = new Date(value);
		if (isNaN(date.getTime())) {
			return "Ugyldig dato";
		}
		const day = ["Søndag", "Mandag", "Tirsdag", "Onsdag", "Torsdag", "Fredag", "Lørdag"][date.getDay()];
		if (strict) {
			return `${day} ${date.getFullYear()}-${zfill(date.getMonth() + 1)}-${zfill(date.getDate())} ${zfill(date.getHours())}:${zfill(date.getMinutes())}`;
		} else {
			const year = new Date().getFullYear() != date.getFullYear() ? date.getFullYear() : "";
			const month = ["januar", "februar", "mars", "april", "mai", "juni", "juli", "august", "september", "oktober", "november", "desember"][date.getMonth() - 1];
			return `${day} ${date.getDate()}. ${month} ${year} kl ${zfill(date.getHours())}:${zfill(date.getMinutes())}`;
		}
	};
	const validate = () => {
		notEmpty("Navn", newEvent.name);
		notEmpty("Hvor", newEvent.location.name);
		notEmpty("Info", newEvent.info);
		if (newEvent.ctf.url) notEmpty("CTF Navn", newEvent.ctf.name);
		notURL("Hvor link", newEvent.location.url);
		notURL("Registrer link", newEvent.register_url);
		notURL("CTF link", newEvent.ctf.url);
		const dateError = parseDate(newEvent.date);
		if (dateError.includes("Ugyldig")) {
			throw dateError;
		}
	};
	const addNew = async () => {
		try {
			validate();
		} catch (err) {
			error = err;
			return;
		}
		error = "";
		newEvent.date = new Date(newEvent.date);
		const res = await fetch("/api/events", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(newEvent),
		});
		if (res.status == 200) {
			newEvent = { ...newEventDefault };
			const data = await fetch("/api/events");
			events = await data.json();
		} else {
			error = "Unable to save event: " + res.statusText;
		}
	};
</script>

<svelte:head>
	<title>Arrangementer</title>
</svelte:head>
<main>
	<h1>Arrangementer</h1>
	<p>Her finner du oversikt over våre fremtidige arrangementer.</p>
	<p>
		<b>NB Fysiske arrangementer:</b>
		<br />
		På grunn av smittesporing trenger vi å registrere alle som ønsker å delta på fysiske arrangementer.
		<br />
		Vennligst meld deg på registreringslenken før du møter opp til et arrangement. Det er ingen tidsfrist for påmelding, så lenge du melder deg på før du fysisk møter opp.
		<br />
		All data blir slettet 2 uker etter arrangement.
	</p>
	{#if $user?.roles?.includes("Styret")}
		<div class="center">
			<ToggleIcon bind:value={showNew} />
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
				<label><span class="col">Når: </span> <input type="text" bind:value={newEvent.date} /> {parseDate(newEvent.date)}</label>
				<label><span class="col">CTF Navn:</span> <input type="text" bind:value={newEvent.ctf.name} /></label>
				<label><span class="col">CTF Link:</span> <input type="text" bind:value={newEvent.ctf.url} /></label>
				<label><span class="col">Info:</span> <textarea rows="3" bind:value={newEvent.info} /></label>
				<label><span class="col">Skjult:</span> <input type="checkbox" bind:value={newEvent.hidden} bind:checked={newEvent.hidden} /></label>
				<span class="col" /> <button on:click|preventDefault={addNew}>{newEvent._id ? "Oppdater" : "Legg til"}</button>
			</form>
		{/if}
	{/if}
	<ul>
		{#each events as event}
			<li class="card">
				{#if $user?.roles?.includes("Styret")}
					<div class="buttons">
						{#if event.hidden}
							<Button disabled icon={FaEyeSlash} />
						{/if}
						<Button submit={() => (newEvent = event) && (showNew = true)} icon={FaCog} />
					</div>
				{/if}
				<h3>{event.name}</h3>
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
							<td>{parseDate(event.date, false)}</td>
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
		background-color: rgba(255, 255, 255, 0.1);
		padding: 0.5em;
		margin-bottom: 1em;
		border: 2px solid rgba(0, 0, 0, 0);
		transition: border 0.2s;
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
	.center {
		display: flex;
		flex-direction: row;
		justify-content: center;
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

	input:not([type="checkbox"]),
	textarea,
	button {
		width: 15em;
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
