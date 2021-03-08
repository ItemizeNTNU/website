<script>
	const events = [
		{
			hva: 'UTCTF',
			hvor: 'Discord',
			når: 'Lørdag 13. mars 12:00',
			ctf: {
				navn: 'UTCTF',
				link: 'https://ctftime.org/event/1177',
			},
			info:
				'Vi deltar på UTCTF. Det blir foodora til alle som deltar 🍔. Grunnet nasjonale tiltak blir opplegget digitalt over Discord.',
		},
		{
			hva: 'UTCTF gjennomgang',
			hvor: 'Discord',
			når: 'Torsdag 18. mars 18:15',
			info: 'Gjennomgang av oppgaver og løsninger til UTCTF.',
		},
		{
			hva: 'CTF med EY',
			hvor: 'Teams/Discord',
			når: 'Torsdag 25. mars 17:15',
			info: 'EY hoster CTF for oss. Påmelding kommer.',
		},
		{
			hva: 'CTF med Deloitte',
			hvor: 'TBA',
			når: '8. april',
			info: 'Deloitte hoster sin nasjonale CTF delfinale hos oss. Påmelding kommer.',
		},
	];
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
		På grunn av smittesporing trenger vi å registrere alle som ønsker å delta på fremtidige fysiske arrangementer.
		<br />
		Vennligst meld deg på en av lenkene under før du møter opp til et arrangement. Det er ingen tidsfrist for påmelding,
		så lenge du melder deg på før du fysisk møter opp.
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
						{#if event.påmelding}
							<tr>
								<td>Registrering:</td>
								<td><a href={event.påmelding} target="_blank" rel="noopener noreferrer">her</a></td>
							</tr>
						{/if}
						{#if event.ctf && event.ctf.navn}
							<td>CTF:</td>
							<td>
								{#if event.ctf.link}
									<a href={event.ctf.link} target="_blank" rel="noopener noreferrer">
										{event.ctf.navn}
									</a>
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
					senere 🙃
				</p>
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
</style>
