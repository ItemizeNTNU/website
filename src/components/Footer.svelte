<script>
	import FaFacebookSquare from 'svelte-icons/fa/FaFacebookSquare.svelte';
	import FaInstagram from 'svelte-icons/fa/FaInstagram.svelte';
	import FaDiscord from 'svelte-icons/fa/FaDiscord.svelte';
</script>

<footer>
	<div class="footer-div">
		<div class="logo">
			<img alt="Logo" src="logo.png" />
		</div>
		<div class="contact">
			<h2>Kontakt oss</h2>
			<div>
				<div>
					<h3>E-post</h3>
					<a href="mailto:hack@itemize.no">hack@itemize.no</a>
				</div>
				<div>
					<h3>Lokasjon</h3>
					<ul>
						<li>Itemize NTNU</li>
						<li>c/o IIK NTNU</li>
						<li>O.S. Bragstads plass 2A</li>
						<li>7034 TRONDHEIM</li>
					</ul>
				</div>
				<div>
					<h3>Organisasjonsnummer</h3>
					<p>925 906 824</p>
				</div>
			</div>
		</div>
		<div class="socials">
			<div>
				<a href="https://www.facebook.com/ItemizeNTNU/">
					<span>
						<FaFacebookSquare />
					</span>
					ItemizeNTNU
				</a>
			</div>
			<div>
				<a href="https://www.instagram.com/itemizentnu/">
					<span>
						<FaInstagram />
					</span>
					itemizentnu
				</a>
			</div>
			<div>
				<a href="https://discord.gg/3NAzCuA">
					<span>
						<FaDiscord />
					</span>
					Itemize NTNU
				</a>
			</div>
		</div>
	</div>
</footer>

<style>
	footer {
		background-color: #1e1e1e;
		padding: 40px 10px;
		color: #e7e7e7;
	}
	.footer-div {
		display: grid;
		grid-template-areas:
			'contact logo socials'
			'contact logo socials'
			'contact logo socials';
		grid-template-columns: repeat(3, minmax(33%, 1fr));
		width: 90%;
		margin: 0 auto;
	}
	.logo {
		width: 100%;
		min-width: 250px;
		max-width: 340px;
		margin: 0 auto;
		grid-row-start: 2;
	}
	.contact {
		grid-area: contact;
		justify-self: center;
	}
	.contact > div {
		display: grid;
	}
	.contact > div > div:not(:last-of-type) {
		margin-bottom: 12px;
	}
	.contact h2 {
		font-weight: 500;
		margin-bottom: 2px;
		color: #fff;
		font-size: 1.3em;
	}
	.contact h3 {
		font-weight: 500;
		margin-bottom: 0;
		font-size: 1.1em;
		color: #fff;
	}
	.contact ul {
		padding: 0;
		margin: 0;
	}
	.contact ul > li {
		list-style-type: none;
	}
	.contact p {
		margin: 0;
	}
	.socials {
		grid-area: socials;
		justify-self: center;
		align-self: center;
		display: grid;
		grid-gap: 12px;
		height: 170px;
	}
	.socials > div {
		display: flex;
		align-items: center;
	}
	.socials > div > a {
		display: flex;
		align-items: center;
	}
	.socials > div > a > span {
		display: inline-block;
		height: 2em;
		vertical-align: sub;
		margin-right: 12px;
		color: #fff;
		transition: 0.4s linear;
	}
	.socials > div > a:hover > span {
		color: var(--green-2);
	}
	@media only screen and (max-width: 820px) {
		.footer-div {
			display: flex;
			justify-content: space-between;
			width: 90%;
			margin: 0 auto;
			flex-wrap: wrap;
		}
		.logo {
			width: 100%;
			display: block;
			min-width: auto;
			max-width: 100%;
			margin: 0 auto 20px auto;
		}
		.logo img {
			width: 100%;
			min-width: 200px;
			max-width: 250px;
			margin: 0 auto;
		}
		.socials {
			align-self: flex-start;
		}
	}
	@media only screen and (max-width: 360px) {
		footer {
			font-size: 3.5vw;
		}
	}
</style>
