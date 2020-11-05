<script>
	const events = [
		{
			hva: 'CTF',
			hvor: 'Digitalt / Discord',
			hvor_link: undefined,
			når: 'Lørdag 7. november 12:00',
			link: undefined,
			ctf: {
				navn: 'KipodAfterFree CTF',
				link: 'https://ctftime.org/event/1133',
			},
			info:
				'På lørdag deltar vi på KipodAfterFree CTF! Grunnet nye rettningslinjer for nasjonale smittevernstiltak så deltar vi kun digitalt denne gangen. Vi kommer til å bruke Discord som kommunikasjonsplatform under CTFen.',
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
								{#if event.hvor_link}
									<a href={event.hvor_link} target="_blank" rel="noopener noreferrer">{event.hvor}</a>
								{:else}{event.hvor}{/if}
							</td>
						</tr>
						<tr>
							<td>Når:</td>
							<td>{event.når}</td>
						</tr>
						{#if event.link}
							<tr>
								<td>Registrering:</td>
								<td><a href={event.link} target="_blank" rel="noopener noreferrer">her</a></td>
							</tr>
						{/if}
						{#if event.ctf && event.ctf.navn}
							<td>CTF:</td>
							<td>
								{#if event.ctf.link}
									<a
										href={event.ctf.link}
										target="_blank"
										rel="noopener noreferrer">{event.ctf.navn}</a>
								{:else}{event.ctf.navn}{/if}
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
