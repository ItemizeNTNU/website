<script>
	import FaAngleUp from 'svelte-icons/fa/FaAngleUp.svelte';
	import Icon from './Icon.svelte';

	export let value = false;
	export let rotate = 0;

	export let colorOn = 'inherit';
	export let colorOff = 'inherit';

	export let iconOff = FaAngleUp;
	export let iconOn = undefined;

	export let onToggle = undefined;
	const _toggle = () => {
		value = !value;
		onToggle && onToggle(value);
	};
</script>

<button on:click={_toggle} style="--angle: {value ? rotate : 0}deg; color: {value ? colorOn : colorOff}">
	{#if value && iconOn}
		<Icon><slot name="on"><svelte:component this={iconOn} /></slot></Icon>
	{:else}
		<Icon><slot name="off"><svelte:component this={iconOff} /></slot></Icon>
	{/if}
</button>

<style>
	button {
		background: none;
		border: none;
		font-size: 1.5em;
		vertical-align: middle;
		transform: rotate(var(--angle));
		transition: all 400ms ease;
		width: max-content;
		min-width: 1.7em;
	}
</style>
