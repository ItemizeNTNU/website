<script>
	const ctf_info =
		'Capture The Flag er konkurranser som omhandler forskjellige aspekter av IT-sikkerhet, og er en god måte å lære seg noen av de praktiske skilza man bruker som «etisk hacker» 😎\nTrykk for mer info.';
	const ctf_info_url = 'https://ctftime.org/ctf-wtf/';
</script>

<svelte:head>
	<title>Historie</title>
</svelte:head>

<main>
	<h3>Itemize i starten</h3>
	<p>
		Itemize NTNU startet opp oktober 2014 på initiativ fra prof. Stig F. Mjølsnes, som fikk inspirasjonen til denne
		studentaktiviteten fra hans forskningsopphold ved Computer Science Department, UCSB i 2013. Der drives CTF-laget
		<code>Shellphish</code>
		av PhD- og master-studenter. Prof. Kemmerer stiftet denne gruppen i 2005 og han arrangerer også hver år
		<a target="_blank" rel="noopener noreferrer" href="http://ictf.cs.ucsb.edu">iCTF</a>.
	</p>
	<p>
		Formålet ble tenkt slik:
		<br />
		<i>
			Itemize legger til rette for å lære praktisk informasjonssikkerhet på en ustrukturert og morsom måte der vi
			lærer av hverandre, og ved å delta i internasjonale hacker-konkurranser (<a
				target="_blank"
				rel="noopener noreferrer"
				href={ctf_info_url}
				title={ctf_info}>CTFer
			</a></i>).
	</p>
	<p>
		En hovedmålsetting helt fra starten av var å delta og ta respektable plasseringer i internasjonale CTFer av
		forskjellig slag. Vi satte ikke noen adgangsbegrensninger for deltagelse i Itemize, men KomTek-studenter med
		studieretning informasjonssikkerhet dannet kjernegruppen.
	</p>

	<p>
		Vi startet med rundt-bordet-samlinger annenhver tirsdag 17-19 på
		<a
			target="_blank"
			rel="noopener noreferrer"
			href="https://use.mazemap.com/#v=1&zlevel=2&center=10.400791,63.418490&zoom=19.5&campusid=1&sharepoitype=poi&sharepoi=34096">Savannen</a>
		ved IIK-instituttet (den gang ITEM-instituttet). En eller to utnevnte studenter tok ansvar for møteregien. Den
		første itemizeren som tok på seg ansvaret å lede gruppen i 2014 var 5. klasse KomTek-student Thomas Kjøglum.
		Navnet Itemize ble avgjort i et online valg den 18. oktober samme år. Valget stod mellom:
	</p>
	<ul>
		<li><code>NTHack</code></li>
		<li><code>Itemize</code></li>
		<li><code>);DROP TABLE names;--</code></li>
		<li><code>No 0x1B</code></li>
		<li><code>ITEM-Ninjas</code></li>
		<li><code>KomtekSec</code></li>
		<li><code>Th3n3wbiz</code></li>
		<li><code>NUVEN</code></li>
		<li><code>TANUVENTÅ</code></li>
	</ul>
	<p>
		Resultatet finnes fremdeles
		<a
			target="_blank"
			rel="noopener noreferrer"
			href="https://docs.google.com/forms/d/1nPZI9Ge39MJ3ITlTXTmkTMZhwmadb7YumGKGKOUo16U/viewanalytics">her</a>.
	</p>

	<p>
		En typisk samling i 2014 var på 18 til 23 studenter, de var fra helt nybegynnere til erfarne og fra alle
		årskull. Selvfølgelig med pizza-servering, som i oppstarten ble velvillig sponset av instituttet. Også
		PhD-studenter og postdocs har tidvis deltatt. Instituttet ga vertskap med rom, utstyr, organisering og regnskap,
		og donerte et par gamle servermaskiner og et lite rom ved siden av studentlabben på Sahara.
	</p>

	<p>
		Vi har deltatt på CTFer som synes aktuelle for oss, dersom mange nok har hatt tid, gjerne gjennom en helg, slik
		at vi kunne danne et godt team for eventen. I etterkant hold vi møter hvor vi i gikk gjennom write-ups fra
		CTFer, med fokus på å forstå utfordringer vi støtte på underveis og lære av oppgavene vi brynet oss på. I
		tillegg kunne medlemmer fortelle om noe de hadde undersøkt, nye teknikker de hadde lært seg og som de ønsket å
		dele med resten av gruppen.
	</p>

	<p>
		På oppfordring fra instituttlederen sendte vi en søknad om driftsstøtte fra KID-nettverket i november 2014, hvor
		vi fikk raskt og positivt svar og tydelig interesse fra bedriftsmedlemmene IBM, Accenture og Bekk. Vi fikk også
		samtidig en forespørsel om å bidra med å arrangere en CTF til en Cybersecurity kveld i 2015. Det ble super
		deltagelse med over 160 studenter. Oppgaver ble utformet av Itemize-medlemmer i ukene før. Oppgavene gav en
		lavterskel smakebit på CTF hvor vi belyste noen enkle og vanlige sikkerhetsproblemer. Suksessen fra 2015 ble
		gjentatt i 2016 med like stor deltakelse og har fortsatt som et årlig samarbeid mellom KID og Itemize frem til
		mest nylig i 2020, nå med oppgaver av alle vanskelighetsgrader fra det enkle til det mer utfordrende.
	</p>

	<p>
		I tillegg til deltakelse i CTFer tenkte vi at det ville vært veldig artig og samlende å ha en mer langsgående
		utviklingsaktivitet som gamle og nye itemizere kan finne attraktivt og spennende. Etter en del diskusjoner ble
		det enighet om å foreslå å sette i gang et internt ”open-ended high performance computing (HPC)” prosjekt basert
		på GPU kort. I 2017 begynte byggingen av server-maskinen Itemizer, med 4 stk Nvidia GTX 1080 GPU kort. Itemizer
		har videre vært i flittig bruk av medlemmer mest for passord hashing, men også andre mindre HPC prosjekter og
		analyser av data og passorddumper.
	</p>

	<p><i>5. oktober 2020. <br /> Revidert nettversjon 20. oktober 2020</i></p>
</main>
