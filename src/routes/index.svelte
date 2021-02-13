<script>
	import Logo from '../components/Logo.svelte';
	import { onMount } from 'svelte';

	const ctf_info =
		'Capture The Flag er konkurranser som omhandler forskjellige aspekter av IT-sikkerhet, og er en god måte å lære seg noen av de praktiske skilza man bruker som «etisk hacker» 😎\nTrykk for mer info.';
	const ctf_info_url = 'https://ctftime.org/ctf-wtf/';

	const info = (e) => {};

	let brainfuck = '';

	let chars =
		'++++++++[->++++++++<]>+++++++++.<++++++[->++++++<]>+++++++.<+++[->---<]>------.++++++++.----.<++++[->++++<]>+.<++++[->----<]>-----.<++++++++[->--------<]>-----.<++++++[->++++++<]>++++++++++.++++++.------.+++++++.<++++++++[->--------<]>--------.---.>';
	chars = chars.repeat(4);
	let i = 0;
	const writeNext = () => {
		const c = chars.charAt(i);
		brainfuck += c;
		i = (i + 1) % chars.length;
		if (brainfuck.length < 3000) {
			setTimeout(writeNext, (30 + Math.pow(Math.random(), 2) * 100 + Math.pow(Math.random(), 6) * 500) * 0.3);
			// setTimeout(writeNext, 1);
		}
	};
	onMount(() => {
		writeNext();
	});
</script>

<svelte:head>
	<title>Itemize NTNU</title>
</svelte:head>

<div class="logo">
	<Logo />
</div>

<div class="background">{brainfuck}</div>

<div class="content">
	<!-- Info -->
	<p>
		Vi er en studentorganisasjon ved NTNU Trondheim som digger informasjonssikkerhet og CTFer.
		<!-- Vi har drop-in med pizza og hacking hver <span class="highlight">onsdag 16:00</span> og ut kvelden. -->
	</p>
	<p>
		Er du interessert i informasjonssikkerhet?
		<br />
		Vil du lære mer om snill hacking?
		<br />
		Liker du å holde på med
		<a target="_blank" rel="noopener noreferrer" href={ctf_info_url} title={ctf_info}>CTFer</a>
		?
		<br />
		Vil du være med på NTNU Trondheim's CTF lag?
		<span class="heavy">
			
			<a href="https://discord.gg/Et9cCKnyg9" target="_blank" rel="noopener noreferrer" style=""><button>Bli med på Discorden her!</button></a>
		</span>
	</p>
	<p>
		Du kan finne en liste over fremtidige arrangementer
		<a href="/registrering">her</a>.
		<br/>
		Følg oss også gjerne på 
		<a target="_blank" rel="noopener noreferrer" href="https://www.facebook.com/ItemizeNTNU">facebook</a>.
	</p>
</div>

<style>
	.content {
		display: flex;
		align-content: center;
		align-items: center;
		flex-direction: column;
		font-size: 1em;
		padding: 0 1em 0em;
	}

	p {
		text-align: center;
		margin-top: 1em;
		margin-bottom: 1em;
		font-size: 1em;
	}

	h1 {
		margin-top: 1em;
		margin-bottom: 0;
	}

	.heavy {
		font-weight: 900;
		margin: 0.3em;
		display: block;
	}

	.content * {
		max-width: 30em;
	}

	.logo {
		padding: 15vw 5vw 10vw;
	}

	.background {
		opacity: 0.1;
		position: absolute;
		top: 60px;
		left: 0;
		width: 100%;
		height: 20em;
		overflow: hidden;
		z-index: -2;
		line-break: anywhere;
		font-size: 4vw;
	}

	.background::after {
		background: linear-gradient(to bottom, transparent, var(--background) 60%);
		content: '';
		z-index: 0;
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
		height: 22em;
	}

	@media (min-width: 700px) {
		.content {
			font-size: 1.3em;
		}

		h1 {
			margin-top: 3em;
		}
		.logo {
			padding: 7vw 15vw;
		}
		.background {
			font-size: 3vw;
			top: 50px;
		}
		.background::after {
			background: linear-gradient(to bottom, transparent, var(--background) 50%);
		}
	}
	@media (min-width: 1400px) {
		.content {
			font-size: 1.4em;
		}
		.logo {
			padding: 7vw 25vw;
		}
		.background {
			font-size: 2.4vw;
			top: 50px;
		}
	}
</style>
