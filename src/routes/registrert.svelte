<script context="module">
	export async function preload(page, session) {
		if (session.user) {
			this.redirect(302, '/profil');
		}
	}
</script>

<svelte:head>
	<title>Vellykket brukerregistrering!</title>
</svelte:head>

<main>
	<h1>Gratulerer!</h1>
	<p>
		Du har nå registrert deg som medlem av Itemize NTNU og skal ha fått tilsendt en lenke på din e-post adresse for å registrere ett passord for din bruker. Etter registrering kan
		du gå til <a href="/login">login siden</a> eller trykke på hengelåsen oppe i høyre hjørne for å logge inn.
	</p>

	<p>Sjekk gjerne ut din <a href="/profil">profil side</a> for å endre instillinger som visningsnavn eller for å får tilgang til vår Discord-server.</p>
</main>
