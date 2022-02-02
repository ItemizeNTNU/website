<script>
	import { goto } from '@sapper/app';
	import Icon from './Icon.svelte';
	export let icon = undefined;
	export let rotate = 0;
	export let disabled = false;
	export let fill = false;
	export let big = false;
	export let error = false;
	export let success = false;
	export let warning = false;
	export let info = false;
	let running = false;
	export let href = '';
	export let submit = href ? async () => await goto(href) : () => {};

	let submitWrapper = async () => {
		if (!disabled) {
			running = true;
			try {
				await submit();
			} catch (err) {
				throw err;
			} finally {
				running = false;
			}
		}
	};
	export let title = undefined;
</script>

<button on:click|preventDefault={submitWrapper} class:fill class:big class:error class:warning class:success class:info class:disabled={disabled || running} {title}>
	<span>
		<slot />
	</span>
	{#if icon}
		<Icon {rotate}><svelte:component this={icon} /></Icon>
	{/if}
</button>

<style>
	button {
		line-height: 1;
		padding: 0.25em;
		min-height: 1.8em;
		min-width: 1.8em;
		width: auto;
	}
	.disabled {
		cursor: default;
		background-color: #0004;
		color: #aaa;
	}
	.fill {
		width: 100%;
		margin: 0;
	}
	.big {
		padding: 0.5em;
	}
</style>
