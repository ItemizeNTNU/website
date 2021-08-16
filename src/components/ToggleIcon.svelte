<script>
	import FaAngleDown from 'svelte-icons/fa/FaAngleDown.svelte';
	import FaAngleUp from 'svelte-icons/fa/FaAngleUp.svelte';
	import Icon from './Icon.svelte';

	export let value = false;
	export let rotate = 180;
</script>

<button on:click={() => (value = !value)} style="--angle: {value ? rotate : 0}deg;">
	{#if value && rotate == 0}
		<Icon><slot name="on"><FaAngleDown /></slot></Icon>
	{:else}
		<Icon><slot name="off"><FaAngleUp /></slot></Icon>
	{/if}
</button>

<style>
	button {
		color: inherit;
		background: none;
		border: none;
		font-size: 1.5em;
		vertical-align: middle;
		transform: rotate(var(--angle));
		transition: transform 400ms ease;
	}
</style>
