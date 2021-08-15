<script>
	import { user } from "../utils/stores";
	import FaLock from "svelte-icons/fa/FaLock.svelte";
	import FaSignOutAlt from "svelte-icons/fa/FaSignOutAlt.svelte";
	import FaUser from "svelte-icons/fa/FaUser.svelte";
	import Icon from "./Icon.svelte";

	export let segment;
	const sider = ["hjem", "om-itemize", "historie", "for-bedrifter", "arrangementer", "ressurser"];
	const home = sider.shift();
</script>

<nav>
	<ul>
		<li><a aria-current={segment === undefined ? "page" : undefined} href="."> {home} </a></li>
		{#each sider as side}
			<li>
				<a aria-current={segment === side ? "page" : undefined} href={side}> {side.replace(/-|_/g, " ")} </a>
			</li>
		{/each}
	</ul>
	<ul>
		{#if $user}
			<li><a title="Profil" href="/profil"><Icon><FaUser /></Icon></a></li>
			<li><a title="Logg ut" href="/logout"><Icon><FaSignOutAlt /></Icon></a></li>
		{:else}
			<li><a title="Logg inn" href="/login"><Icon><FaLock /></Icon></a></li>
		{/if}
	</ul>
</nav>

<style>
	nav {
		border-bottom: 1px solid rgba(60, 179, 79, 0.1);
		font-weight: 300;
		padding: 0 1em;
		display: flex;
		flex-direction: row;
		justify-content: space-between;
	}

	ul {
		margin: 0;
		padding: 0;
	}

	/* clearfix */
	ul::after {
		content: "";
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
		content: "";
		width: calc(100% - 1em);
		height: 2px;
		background-color: rgb(60, 179, 79);
		display: block;
		bottom: -1px;
	}

	a,
	span {
		text-decoration: none;
		color: inherit;
		padding: 1em 0.5em;
		display: block;
	}
</style>
