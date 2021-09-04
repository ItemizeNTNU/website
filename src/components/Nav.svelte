<script>
	import { user } from '../utils/stores';
	import FaLock from 'svelte-icons/fa/FaLock.svelte';
	import FaSignOutAlt from 'svelte-icons/fa/FaSignOutAlt.svelte';
	import FaUser from 'svelte-icons/fa/FaUser.svelte';
	import Icon from './Icon.svelte';

	export let segment;
	const sider = ['~/', '/om-itemize/', '/historie/', '/for-bedrifter/', '/arrangementer/', '/ressurser/'];
	const home = sider.shift();

	let activatedBurger = false;
	const toggleBurger = () => (activatedBurger = !activatedBurger);
</script>

<nav>
	<button on:click={toggleBurger} class={`hamburger ${activatedBurger ? 'active' : ''}`}>
		<span />
		<span />
		<span />
	</button>
	<ul>
		<li><a aria-current={segment === undefined ? 'page' : undefined} href="."> {home} </a></li>
		{#each sider as side}
			<li>
				<a aria-current={`/${segment}/` === side ? 'page' : undefined} href={side}> {side.replace(/-|_/g, '_')} </a>
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
		height: 55px;
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
		bottom: 1px;
	}
	a {
		text-decoration: none;
		color: inherit;
		padding: 1em 0.5em;
		display: block;
	}
	.hamburger {
		display: none;
	}
	@media only screen and (max-width: 640px) {
		nav {
			justify-content: flex-end;
			padding-right: 75px;
		}
		ul:first-of-type {
			display: block;
			position: absolute;
			background-color: #202020;
			right: 20px;
			top: 20px;
			z-index: 5;
			padding: 10px 50px 10px 20px;
			border-radius: 5px;
			transition: 0.7s all;
			transform: translateX(150%);
		}
		li {
			display: block;
			float: initial;
		}
		.hamburger {
			display: block;
			position: absolute;
			top: 2.5px;
			right: 10px;
			width: 50px;
			height: 50px;
			background-color: #1e1e1e;
			z-index: 6;
			transition: 1s all;
		}

		.hamburger span {
			position: absolute;
			display: block;
			width: 25px;
			height: 3px;
			left: 10px;
			top: 23px;
			background-color: #dddddd;
			transition: all 0.5s;
		}
		.hamburger span:first-of-type {
			top: 15px;
		}
		.hamburger span:last-of-type {
			top: 31px;
			width: 20px;
		}
		.hamburger.active {
			background-color: #dddddd;
			border-radius: 50%;
		}
		.hamburger.active span {
			background-color: #302b2e;
		}
		.hamburger.active + ul:first-of-type {
			transform: translateX(0);
		}
		.hamburger.active span:last-of-type {
			width: 25px;
		}

		.hamburger:not(.active):hover {
			border-color: rgba(0, 0, 0, 0);
			background-color: rgba(0, 0, 0, 0.6);
		}
		.hamburger:not(.active):hover span:last-of-type {
			width: 25px;
		}
	}
</style>
