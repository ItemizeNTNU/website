<script>
	export let segment;
</script>

<style>
	nav {
		border-bottom: 1px solid rgba(60, 179, 79, 0.1);
		font-weight: 300;
		padding: 0 1em;
	}

	ul {
		margin: 0;
		padding: 0;
	}

	/* clearfix */
	ul::after {
		content: '';
		display: block;
		clear: both;
	}

	li {
		display: block;
		float: left;
	}

	[aria-current] {
		position: relative;
		display: inline-block;
	}

	[aria-current]::after {
		position: absolute;
		content: '';
		width: calc(100% - 1em);
		height: 2px;
		background-color: rgb(60, 179, 79);
		display: block;
		bottom: -1px;
	}

	a {
		text-decoration: none;
		color: inherit;
		padding: 1em 0.5em;
		display: block;
	}
</style>

<nav>
	<ul>
		<li><a aria-current={segment === undefined ? 'page' : undefined} href="."> hjem </a></li>
		<li><a aria-current={segment === 'historie' ? 'page' : undefined} href="historie"> historie </a></li>
		<li>
			<a aria-current={segment === 'for-bedrifter' ? 'page' : undefined} href="for-bedrifter"> for bedrifter </a>
		</li>
		<li>
			<a aria-current={segment === 'registrering' ? 'page' : undefined} href="registrering"> registrering </a>
		</li>
	</ul>
</nav>
