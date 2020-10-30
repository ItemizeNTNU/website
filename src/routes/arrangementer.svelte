<script>
	const events = [
		{
			hva: 'NACTF',
			hvor: 'EL3',
			hvor_link: 'http://use.mazemap.com/?v=1&campuses=ntnu&sharepoitype=identifier&sharepoi=325-G102',
			når: 'Lørdag 13. oktober 12:00',
			link: 'https://forms.gle/Tstw4FyFAjuKuw3WA',
			ctf: {
				navn: 'NACTF',
				link: 'https://ctftime.org/event/1157',
			},
			info:
				'På lørdag deltar vi på NACTF! Den er mer rettet mot nybegynnere, så vi håper det blir lærerikt for alle og enhver.',
		},
	];
</script>

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
</style>

<svelte:head>
	<title>Arrangementer</title>
</svelte:head>
<main>
	<h1>Arrangementer</h1>
	<p>Her finner du oversikt over våre fremtidige arrangementer.</p>
	<p>På grunn av smittesporing trenger vi å registrere alle som ønsker å delta på fremtidige arrangementer.</p>
	<p>
		Vennligst meld deg på en av lenkene under før du møter opp til et arrangement. Det er ingen tidsfrist for
		påmelding, så lenge du melder deg på før du fysisk møter opp.
	</p>
	<ul>
		{#each events as event}
			<li>
				<h3>{event.hva}</h3>
				<table>
					<tbody>
						<tr>
							<td>Hvor:</td>
							<td>
								<a href={event.hvor_link} target="_blank" rel="noopener noreferrer">{event.hvor}</a>
							</td>
						</tr>
						<tr>
							<td>Når:</td>
							<td>{event.når}</td>
						</tr>
						<tr>
							<td>Registrering:</td>
							<td><a href={event.link} target="_blank" rel="noopener noreferrer">her</a></td>
						</tr>
						{#if event.ctf}
							<td>CTF:</td>
							<td>
								<a href={event.ctf.link} target="_blank" rel="noopener noreferrer">{event.ctf.navn}</a>
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
				<p>
					Det ser ikke ut som vi har noen planlagte arrangementer annonsert enda. Kom gjerne tilbake igjen
					senere 🙃.
				</p>
			</li>
		{/each}
	</ul>
</main>
