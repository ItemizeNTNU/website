<script>
	import { goto } from "@sapper/app";
	import Icon from "./Icon.svelte";
	export let icon;
	export let disabled = false;
	let running = false;
	export let href = "";
	export let submit = href ? async () => await goto(href) : () => {};
	let submitWrapper = async () => {
		if (!disabled) {
			running = true;
			await submit();
			running = false;
		}
	};
	let content;
</script>

<button on:click|preventDefault={submitWrapper} class:clear={icon && !content?.textContent} class:disabled={disabled || running}>
	{#if icon}
		<Icon><svelte:component this={icon} /></Icon>
	{/if}
	<span bind:this={content}>
		<slot />
	</span>
</button>

<style>
	button {
		vertical-align: middle;
		line-height: 1;
		padding: 0.5em;
	}
	.disabled {
		cursor: default;
		background-color: #0004;
		color: #aaa;
	}
	.clear {
		background: none;
		width: auto;
		min-height: 2.325em;
		min-width: 2.325em;
	}
</style>
