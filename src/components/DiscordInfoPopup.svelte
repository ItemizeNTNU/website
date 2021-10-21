<script>
	import { onMount } from 'svelte';
	export let loggedIn;

	let discordInfoPopup, discordInfoButton;
	let infoChecked = false;
	const hidePopup = (bool) => {
		discordInfoPopup.classList.remove('visible');
		sessionStorage['hideInfoOnSession'] = true;
		if (bool) localStorage['hideInfoOnSession'] = true;
	};
	const initDiscordPopup = () => {
		console.log(loggedIn);
		if (!sessionStorage['hideInfoOnSession'] && !localStorage['hideInfoOnSession'] && loggedIn) {
			discordInfoPopup.classList.add('visible');
			setTimeout(() => {
				hidePopup(infoChecked);
			}, 25000);
		}
		discordInfoButton.onclick = (e) => {
			e.preventDefault();
			hidePopup(infoChecked);
		};
	};
	onMount(() => initDiscordPopup());
</script>

<div class="discordInfoPopup" bind:this={discordInfoPopup}>
	<p>Heisann!</p>
	<p>Velkommen som ny medlem i Itemize!</p>
	<p>
		For medlemmer bruker vi Discord server som hovedkommunikasjonnkanal. Bli med her: <a href="https://discord.com/invite/gWJdXbW8Sg">https://discord.com/invite/gWJdXbW8Sg</a>
	</p>
	<p>Du kan linke Discord brukeren din med Itemize sin på profil siden: <a href="https://itemize.no/profil">https://itemize.no/profil</a></p>
	<p>
		<input type="checkbox" id="discordChkbox" bind:checked={infoChecked} />
		<label for="discordChkbox"> Ikke vis denne meldingen igjen </label>
	</p>
	<button bind:this={discordInfoButton}>Lukk meldingen</button>
</div>

<style>
	input[type='checkbox'] {
		margin-top: 0.23em;
	}
	button {
		vertical-align: middle;
		line-height: 1;
		padding: 0.5em;
		margin-top: 10px;
		width: 100%;
	}
	.discordInfoPopup {
		position: fixed;
		border-radius: 6px;
		box-shadow: 0 16px 24px 2px rgba(0, 0, 0, 0.14), 0 6px 30px 5px rgba(0, 0, 0, 0.12), 0 8px 10px -5px rgba(0, 0, 0, 0.2);
		bottom: 10px;
		left: 10px;
		z-index: 1000;
		width: calc(100% - 60px);
		max-width: 400px;
		transition: 1s all;
		transform: translateX(-110%);
		padding: 1em;
		background: #262626;
		border: 2px solid #444;
		font-size: 0.95em;
	}
	:global(.discordInfoPopup.visible) {
		transform: translateX(0) !important;
	}
	.discordInfoPopup > p {
		margin: 2px 0;
	}
</style>
