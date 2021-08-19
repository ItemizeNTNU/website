<script>
	import People from '../components/People.svelte';

	const people = [
		{
			name: 'Håvard Borgen Myrekrok',
			position: 'Bedriftskontakt',
			mail: 'havard.myrekrok@itemize.no'
		},
		{
			name: 'Erik Borgeteien Hansen',
			position: 'Bedriftskontakt',
			mail: 'erik.borgeteien.hans@itemize.no'
		}
	];
</script>

<svelte:head>
	<title>For Bedrifter</title>
</svelte:head>

<main>
	<h1>For Bedrifter</h1>
	<p>
		Itemize NTNU er NTNU Trondheims CTF-lag og en åpen interesseorganisasjon for alle ved NTNU med interesse for IT-sikkerhet. Vi ønsker å styrke den faglige kompetansen til våre
		medlemmer. Dette oppnår vi gjennom å holde kurs og CTF-konkurranser, og vi samarbeider gjerne med bedrifter som vil bidra til kompetansehevingen til våre medlemmer. Her er
		mulighetene mange, og vi er åpne for innspill til format på arrangement.
	</p>

	<h2>Hva vi kan bistå med</h2>
	<p>
		Under et arrangement kan vi bistå med digital kommunikasjonskanal, profilering av arrangementet overfor våre medlemmer, og påmelding. Ved behov kan vi også bidra med
		CTF-oppgaver eller andre praktiske gjøremål tilknyttet arrangementet.
	</p>

	<h2>Kontakt</h2>
	<p>
		Er du en bedrift eller annen ekstern part som ønsker å gjennomføre et samarbeid med oss og våre medlemmer, eller har noe du lurer på? Ta kontakt med oss på <a
			href="mailto:bedrift@itemize.no">bedrift@itemize.no</a
		>.
	</p>

	<People {people} />
</main>
