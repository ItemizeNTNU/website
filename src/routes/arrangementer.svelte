<script>
	const events = [
		{
			hva: 'UMDCTF 2021',
			hvor: 'Discord',
			når: 'Lørdag 17. april kl 12:00',
			ctf: {
				navn: 'UMDCTF 2021',
				link: 'https://ctftime.org/event/1288',
			},
			info: 'Vi deltar på UMDCTF. Arrangementet foregår digitalt på Discord.',
		},
		{
			hva: 'HeroCTF v3',
			hvor: 'TBA',
			når: 'Lørdag 24. april kl 12:00',
			ctf: {
				navn: 'HeroCTF v3',
				link: 'https://ctftime.org/event/1284',
			},
			info: 'Vi deltar på HeroCTF. Vi håper på å møtes fysisk hvis det lar seg gjennomføre.',
		},
		{
			hva: 'Itemize-kveld',
			hvor: 'TBA',
			når: 'Torsdag 29. april kl 18:15',
			info: 'Semesteravslutning',
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
