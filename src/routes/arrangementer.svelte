<script>
	const events = [
		{
			hva: 'Itemize-kveld',
			hvor: 'EL2',
			påmelding: 'https://cloud.itemize.no/apps/forms/CJbXpG4JtQKDYWGD',
			hvor_link: 'https://link.mazemap.com/3dDBJomf',
			når: 'Torsdag 29. april kl 17:15',
			info:
				'Siste Itemize-kveld dette semesteret. Det blir mini-kurs til HackTheBox, prosjektjobbing og ellers sosialt og god stemning med pizza! 🥳',
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
		På grunn av smittesporing trenger vi å registrere alle som ønsker å delta på fysiske arrangementer.
		<br />
		Vennligst meld deg på registreringslenken før du møter opp til et arrangement. Det er ingen tidsfrist for påmelding,
		så lenge du melder deg på før du fysisk møter opp.
		<br />
		All data blir slettet 2 uker etter arrangement.
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
