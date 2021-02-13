<script>
    const board = [
        {
            name: 'Aleksander Styrmoe',
            position: 'Leder',
            mail: 'leder@itemize.no',
        },
        {
            name: 'Sigurd Varhaugvik',
            position: 'Nestleder',
            mail: 'nestleder@itemize.no',
        },
        {
            name: 'Kristian in\'t Veld',
            position: 'Driftsansvarlig',
            mail: 'drift@itemize.no',
        },
        {
            name: 'Anna Fridtun Aarekol',
            position: 'Prosjektansvarlig',
            mail: 'prosjekt@itemize.no',
        },
        {
            name: 'Håkon Tjomsland',
            position: 'CTF- og arrangementsansvarlig',
            mail: 'arrangement@itemeize.no',
        },
        {
            name: 'Knut Formo Buene',
            position: 'PR- og rekrutteringsansvarlig',
            mail: 'pr@itemize.no',
        },
    ];
</script>

<svelte:head>
	<title>Om Itemize</title>
</svelte:head>

<main>
    <h1>Om Itemize</h1>

    <p>
        Itemize NTNU er en frivillig interesseorganisasjon for studenter og ansatte ved NTNU Trondheim som liker eller vil lære om informasjonssikkerhet og hacking.
        I vedtektene til Itemize NTNU står det også at Itemize skal være med på å fremme god sikkerhetskultur i Norge.
        Dette gjør vi ved å delta på både nasjonale og internasjonale CTFer jevnlig.
    </p>
    <p>
        Driften til Itemize er prosjektbasert, noe som betyr at medlemmer kan velge å engasjere seg i kortvarige prosjekter.
        Dette sikrer at Itemize utvikler seg videre.
        Det er selvfølgelig også mulig å være medlem uten å være med på prosjekter, men heller bare være med på CTFer og andre arrangementene man har mulighet til.
    </p>
    <p>
        Itemize NTNU er tilknyttet Institutt for informasjonssikkerhet og kommunikasjonsteknologi og får store deler av støtten til driften derifra.
    </p>
    <p>
        Vi har også en alumniordning for tidligere medlemmer, slik at de kan være med videre på å hjelpe fremtidens sikkerhetsinteresserte.
    </p>
    <p>
        For medlemmer bruker vi <a href="https://discord.gg/Et9cCKnyg9" target="_blank" rel="noopener noreferrer">Discord</a> som hovedkommunikasjonskanal.
    </p>

    <h2>Kontakt</h2>

    <p>
        Vi tar imot innmeldinger, spørsmål og andre henvendelser på mail: <a href="mailto:hack@itemize.no">hack@itemize.no</a>
    </p>
    <p>
        <i>Postadresse:</i><br>
        Itemize NTNU<br>
        c/o Institutt for informasjonssikkerhet og kommunikasjonsteknologi<br>
        O.S. Bragstads plass 2A<br>
        7034 TRONDHEIM
    </p>
    <p>
        <i>Org.nr:</i> 925 906 824
    </p>
    <p>
        Fakturaer sendes til <a href="mailto:faktura@itemize.no">faktura@itemize.no</a>.
    </p>

    <h2>Styret</h2>

    <ul>
        {#each board as person}
            <li class="boxes">
                <b>{person.position}</b>: {person.name}
                <span class='right'><a href='mailto:{person.mail}'>{person.mail}</a></span>
            </li>
        {/each}
    </ul>

</main>

<style>
	.boxes {
		list-style-type: none;
		background-color: rgba(255, 255, 255, 0.1);
		padding: 0.5em;
		margin-bottom: 1em;
		border: 2px solid rgba(0, 0, 0, 0);
		transition: border 0.2s;
	}

	.boxes:hover {
		border: 2px solid rgba(255, 255, 255, 0.05);
		transition: border 0s;
	}

	ul {
		padding: 0;
	}

    .right {
        float: right;
    }
</style>
