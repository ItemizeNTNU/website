<script>
	import { onMount } from 'svelte';

	let path = '/ressurser';

	onMount(() => {
		path = window.location.pathname;
	});
</script>

<style>
	li p {
		margin-top: 0.2em;
		margin-bottom: 0.2em;
	}
	li {
		margin-bottom: 1.5em;
	}

	ul.overview li {
		margin-bottom: 0;
	}
	h2 {
		margin-top: 2em;
	}
</style>

<svelte:head>
	<title>Ressurser</title>
</svelte:head>

<main>
	<h1>Ressurser:</h1>
	<ul class="overview">
		<li><a href="{path}#ctfer">CTFer</a></li>
		<li><a href="{path}#youtube-kalaer">YouTube Kanaler</a></li>
		<li><a href="{path}#verktøy">Verktøy</a></li>
	</ul>
	<h2 id="ctfer">CTFer:</h2>
	<ul>
		<li>
			<a href="https://play.picoctf.org/practice" target="_blank" rel="noopener noreferrer">PicoCTF</a>
			<p>
				PicoCTF sin practice gym er et fint sted å begynne med introduksjonsoppgaver for web, kryptografi,
				reverse enginering, forensics og linux kommandolinje.
			</p>
		</li>
		<li>
			<a href="https://ctf.hacker101.com" target="_blank" rel="noopener noreferrer">Hacker 101</a>
			<p>CTF oppgaver laget av HackerOne. Har mange ulike oppgaver og fint introduksjonsnivå.</p>
		</li>
		<li>
			<a href="https://cryptohack.org" target="_blank" rel="noopener noreferrer">Cryptohack</a>
			<p>Mange ulike CTF oppgaver innen kryptografi.</p>
		</li>
		<li>
			<a href="https://crackmes.one" target="_blank" rel="noopener noreferrer">Crackmes</a>
			<p>Crackmes er en samnling med reverse enginering oppgaver av ulike slag.</p>
		</li>
		<li>
			<a href="https://www.hackthebox.eu" target="_blank" rel="noopener noreferrer">Hack The Box</a>
			<p>
				Hack The Box (HTB) har ulike virtuelle maskiner eller bokser hvor du gjør større deler av mer reell
				penetrasjonstesting. Alle boksene pleier å følge prosessen med å starte ved scanning, få kjørt kode som
				bruker, for så å eskalere opp til system/root bruker.
			</p>
			<p>
				Fordel med HTB over andre CTFer er at ting gjerne føles litt mer reelt. Oppgaver og bokser er gjerne
				litt mer omfatende enn kun én enkelt CTF oppgave, men mange av boksene kan løses med standard verktøy og
				går gjerne ikke fult så inn i detaljer som andre mer tradisjonelle CTF oppgaver.
			</p>
		</li>
		<li>
			<a href="https://tryhackme.com" target="_blank" rel="noopener noreferrer">Try Hack Me</a>
			<p>
				Alternativ til Hack The Box. Har virtuelle maskiner hvor du gjør hele penetrasjonstesting prosessen med
				scanning, få bruker og eskaler til root/system.
			</p>
		</li>
	</ul>

	<h2 id="youtube-kalaer">YouTube Kanaler</h2>
	<ul>
		<li>
			<a
				href="https://www.youtube.com/channel/UClcE-kVhqyiHCcjYwcpfj9w"
				target="_blank"
				rel="noopener noreferrer">LiveOverflow</a>
			<p>
				LiveOverflow lager høykvalitets videoer hvor han presenterer temaer, sårbarheter og alt annen interesant
				innen infosec. Han går også gjennom CTF oppgaver og fokuserer mer på unike og lærerike oppgaver i stedet
				for kvantitet.
			</p>
		</li>
		<li>
			<a href="https://www.youtube.com/c/JohnHammond010/" target="_blank" rel="noopener noreferrer">John Hammond</a>
			<p>Youtuber som går gjennom mye CTFer og TryHackMe bokser. Viser mye ulike verktøy og metoder.</p>
		</li>
		<li>
			<a href="https://www.youtube.com/c/ippsec/" target="_blank" rel="noopener noreferrer">IppSec</a>
			<p>
				IppSec går gjennom alle Hack The Boks oppgaver som blir lagt ut etter de ikke lenger er "aktive". Han
				går også jennom andre verktøyer og metoder fra tid til annen. Han viser også hele prosessen med å gå
				gjennom boksen.
			</p>
			<p>
				Sjekk også ut
				<a href="https://ippsec.rocks" target="_blank" rel="noopener noreferrer">ippsec.rocks</a>
				hvor du kan søke gjennom alle videoene han har gått gjennom etter spesifikke verktøy eller metoder,
				kommer med timestamps til videoene.
			</p>
		</li>
	</ul>

	<h2 id="verktøy">Verktøy</h2>
	<ul>
		<li>
			<a href="https://gchq.github.io/CyberChef/" target="_blank" rel="noopener noreferrer">CyberChef</a>
			<p>
				CyberChef er et online verktøy med over 300 ulike moduler bygget inn. Har alt fra base64 til kryptering
				og fil gjenskjenning. CyberChef har også automatiske moduler samt tillater chaining med moduler etter
				hverandre. Mange CTF oppgaver kan ofte løses ene og alene i CyberChef.
			</p>
		</li>
		<li>
			<a href="https://portswigger.net/burp" target="_blank" rel="noopener noreferrer">Burp Suite</a>
			<p>
				En web proxy for å logge, redigere og injekte forespørsler mellom deg og nettleseren din. Bra verktøy
				for å gjøre manuelle endringer til forespørsler eller gjenta forespørsler flere ganger.
			</p>
		</li>
		<li>
			<a href="https://hashes.com" target="_blank" rel="noopener noreferrer">Hashes</a>
			<p>Online verktøy for å detektere hash type samt søke etter kjente hashes.</p>
		</li>
		<li>
			<a href="https://zxing.org/w/decode.jspx" target="_blank" rel="noopener noreferrer">ZXing Decoder</a>
			<p>Online implementasjon av XZing, et QRcode verktøy som støtter mange ulike 1D og 2D QRkode formater.</p>
		</li>
		<li>
			<a href="https://github.com/Gallopsled/pwntools" target="_blank" rel="noopener noreferrer">pwntools</a>
			<p>
				Pwntools er et CTF toolkit skrevet i python for rask prototyping og tar seg av mye av "boilerplate"
				koden som man skriver hver gang. Pwntools har også en god del kule og nyttige funksjoner som en ofte
				bruker på CTFer, som generering av shellcode payloads eller enkel asynkron håndtering av sockets og
				prosesser.
			</p>
		</li>
		<li>
			<a href="https://godbolt.org/" target="_blank" rel="noopener noreferrer">Compiler Explorer</a>
			<p>
				Online verktøy for å studere assembly bytecode. Støtter en rekke ulike kompilatorer i flere ulike språk.
				Fint for å sammenligne hva kode faktisk kompilerer til.
			</p>
		</li>
		<li>
			<a
				href="https://github.com/DominicBreuker/stego-toolkit"
				target="_blank"
				rel="noopener noreferrer">stego-toolkit</a>
			<p>
				En samling steganography verktøy samlet i en enkel verktøykase. Finnes også et
				<a
					href="https://hub.docker.com/r/dominicbreuker/stego-toolkit"
					target="_blank"
					rel="noopener noreferrer">docker image</a>
				du kan kjøre, så slipper du å installere alle verktøy selv.
			</p>
		</li>
		<li>
			<a href="https://stegonline.georgeom.net/checklist" target="_blank" rel="noopener noreferrer">StegOnline
				Checklist</a>
			<p>En fin liste over ting man bør teste når man går gjennom steganography oppgaver.</p>
		</li>
		<li>
			<a href="https://requestbin.com" target="_blank" rel="noopener noreferrer">requestbin</a>
			<p>
				Lar deg sette opp webhook endpoints for callbacks, fin måte å kunne hente ut informasjon hvis du ikke
				har en egen server tilgjengelig.
			</p>
		</li>
		<li>
			<a
				href="https://www.boxentriq.com/code-breaking/cipher-identifier"
				target="_blank"
				rel="noopener noreferrer">Cipher Identifier</a>
			<p>Verktøy som kan hjelpe å identifisere hvilken type krypteringscipher som har blitt brukt.</p>
		</li>
		<li>
			<a href="https://github.com/apsdehal/awesome-ctf" target="_blank" rel="noopener noreferrer">awesome-ctf</a>
			<p>En samling med ulike verktøy fordelt på kategorier. Inneholder de fleste verktøy det er å anbefale.</p>
		</li>
		<li>
			<a href="https://ctfs.github.io/resources/" target="_blank" rel="noopener noreferrer">CTF Resources</a>
			<p>Et open source github wiki prosjekt som samller mange verktøy og teknologier.</p>
		</li>
	</ul>
</main>
