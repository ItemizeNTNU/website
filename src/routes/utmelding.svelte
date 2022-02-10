<script>
	const to = 'drift@itemize.no';
	const subject = 'Utmelding av Itemize NTNU';
	const body = 'Hei.\n\nJeg øsnker å melde meg ut av Itemize NTNU. Jeg bekrefter at jeg ønsker all bruker informasjon slettet.\n\nHilsen XXXX';
	const mailto = `mailto:${to}?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(body)}`;
</script>

<svelte:head>
	<title>Utmelding</title>
</svelte:head>

<main>
	<h1>Utmelding</h1>

	<p>Itemize er under kontinuerlig utviling og vi er ikke helt i mål med å utarbeide alle systemene våre enda. Et av disse er automatisk system for utmelding.</p>

	<p>
		Enn så lenge ber vi eksisterende medlemmer som ønsker å melde seg ut av Itemize NTNU om å sende e-post til <a href={mailto}>drift@itemize.no</a>. Du må bruke samme
		e-postaddresse som du har registrert brukeren din med. Du trenger ikke å oppgi noe videre informasjon eller begrunnelse utover navn og e-post adresse. Trykk gjerne på
		e-postlenken eller bruk malen nedenfor.
	</p>

	<p>
		Merk at ved å melde deg ut av Itemize NTNU så vil du miste tilgang til alle Itemize NTNU sine systemer, slik som medlems Wiki og Discord. Dine personlie filer på Cloud og
		tilsvarende systemer vil også bli slettet.
	</p>

	<br />
	<hr />
	<br />
	<h2>E-post mal for utmelding:</h2>
	<p>
		<b>To:</b> <code>{to}</code>
		<br />
		<b>Subject:</b> <code>{subject}</code>
		<code class="block">
			{body}
		</code>
	</p>
</main>
