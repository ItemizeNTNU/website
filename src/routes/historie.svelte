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
		Itemize NTNU startet opp oktober 2014 på initiativ fra prof. Stig F. Mjølsnes, som fikk inspirasjonen til denne studentaktiviteten fra hans forskningsopphold ved Computer
		Science Department, UCSB i 2013. Der drives CTF-laget
		<code>Shellphish</code>
		av PhD- og master-studenter. Prof. Kemmerer stiftet denne gruppen i 2005 og han arrangerer også hvert år
		<a target="_blank" rel="noopener noreferrer" href="http://ictf.cs.ucsb.edu">iCTF</a>.
	</p>
	<p>
		Formålet ble tenkt slik:
		<br />
		<i>
			Itemize legger til rette for å lære praktisk informasjonssikkerhet på en ustrukturert og morsom måte der vi lærer av hverandre, og ved å delta i internasjonale
			hacker-konkurranser (<a target="_blank" rel="noopener noreferrer" href={ctf_info_url} title={ctf_info}>CTFer </a></i
		>).
	</p>
	<p>
		En hovedmålsetting helt fra starten av var å delta og oppnå respektable plasseringer i internasjonale CTFer av forskjellig slag. Det ble ikke satt noen adgangsbegrensninger for
		deltagelse i Itemize, men KomTek-studenter med studieretning informasjonssikkerhet dannet kjernegruppen.
	</p>

	<p>
		Gruppen startet med rundt-bordet-samlinger annenhver tirsdag 17-19 på
		<a target="_blank" rel="noopener noreferrer" href="https://use.mazemap.com/#v=1&zlevel=2&center=10.400791,63.418490&zoom=19.5&campusid=1&sharepoitype=poi&sharepoi=34096"
			>Savannen</a
		>
		ved IIK-instituttet (den gang ITEM-instituttet). En eller to utnevnte studenter tok ansvar for møteregien. Den første itemizeren som tok på seg ansvaret å lede gruppen i 2014 var
		5. klasse KomTek-student Thomas Kjøglum. Navnet Itemize ble avgjort i et online valg den 18. oktober samme år. Valget stod mellom:
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
		<a target="_blank" rel="noopener noreferrer" href="https://docs.google.com/forms/d/1nPZI9Ge39MJ3ITlTXTmkTMZhwmadb7YumGKGKOUo16U/viewanalytics">her</a>.
	</p>

	<p>
		En typisk samling i 2014 var på 18 til 23 studenter, bestående av alt fra nybegynnere til erfarne fra alle årskull. Selvfølgelig med pizza-servering, som i oppstarten ble
		velvillig sponset av instituttet. Også PhD-studenter og postdocs har tidvis deltatt. Instituttet ga vertskap med rom, utstyr, organisering og regnskap, og donerte et par gamle
		servermaskiner og et lite rom ved siden av studentlabben på Sahara.
	</p>

	<p>
		Gruppen startet å delta på CTFer. I etterkant ble det holdt gjennomganger av relevante write-ups fra CTFene, med fokus på å forstå utfordringer og lære av hverandre. I tillegg
		kunne medlemmer fortelle om nye konsepter og teknikker de hadde lært.
	</p>

	<p>
		På oppfordring fra instituttlederen sendte Itemize en søknad om driftsstøtte fra KID-nettverket i november 2014, hvor de raskt fikk svar om tydelig interesse fra
		bedriftsmedlemmene IBM, Accenture og Bekk. Itemize fikk også samtidig en forespørsel om å bidra med å arrangere en CTF til en Cybersecurity-kveld i 2015. Arrangementet hadde en
		super deltagelse med over 160 studenter. Oppgaver ble utformet av Itemize-medlemmer i ukene før arrengementet. Oppgavene gav en lavterskel smakebit på CTF, med oppgaver som
		belyste enkle og vanlige sikkerhetsproblemer. Suksessen fra 2015 ble gjentatt i 2016 med like stor deltakelse og har fortsatt som et årlig samarbeid mellom KID og Itemize. I
		2020 ble det laget oppgaver med vanskelighetsgrader fra det enkle til det mer utfordrende.
	</p>

	<p>
		I tillegg til deltakelse i CTFer var det et ønske i gruppen om å organisere mer langsgående utviklingsaktiviteter for medlemmene. Etter en del diskusjoner ble det enighet om å
		foreslå å sette i gang et internt ”open-ended high performance computing (HPC)” prosjekt, basert på GPU kort. I 2017 begynte byggingen av server-maskinen Itemizer, med 4 stk
		Nvidia GTX 1080 GPU kort. Itemizer har videre vært i flittig bruk av medlemmer mest for passord-hashing, men også andre mindre HPC prosjekter og analyser av data og
		passorddumper.
	</p>

	<p><i>5. oktober 2020. <br /> Revidert nettversjon 13. februar 2021</i></p>
</main>
