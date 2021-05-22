<script>
	import Icon from "./Icon.svelte";
	export let submit = () => {};
	export let icon;
	export let disabled = false;
	let submitWrapper = () => {
		if (!disabled) {
			submit();
		}
	};
	let content;
</script>

<button on:click|preventDefault={submitWrapper} class:clear={icon && !content?.textContent} class:disabled>
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
	}
</style>
