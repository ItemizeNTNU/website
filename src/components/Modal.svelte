<script context="module">
	let zIndexCounter = 100;
</script>

<script>
	import FaCheck from 'svelte-icons/fa/FaCheck.svelte';
	import FaTimes from 'svelte-icons/fa/FaTimes.svelte';

	import Button from './Button.svelte';
	import { fade } from 'svelte/transition';

	export let title = '';
	export let show = false;
	export let acceptButton = '';
	export let acceptDisable = false;
	export let closeButton = '';

	$: if (acceptButton && typeof acceptButton != 'string') acceptButton = 'Accept';
	$: if (closeButton && typeof closeButton != 'string') closeButton = 'Close';

	let zIndex;

	$: {
		if (show) {
			zIndex = zIndexCounter++; // svelte-ignore module-script-reactive-declaration
		}
	}

	export let onAccept = () => {};
	export let onClose = () => {};

	const close = (f) => {
		return () => {
			show = false;
			if (f) f();
		};
	};

	let overlay;
	const keyup = (e) => {
		const topOverlay = document.elementFromPoint(0, 0);
		if (topOverlay == overlay) {
			if (e.key == 'Escape') {
				if (!e.target.matches('input, div.textarea[role="textbox"], textarea')) {
					close(onClose)();
				}
			}
		}
	};
	const onShow = (node) => {
		overlay = node;
		document.addEventListener('keyup', keyup);
		return {
			destroy() {
				document.removeEventListener('keyup', keyup);
			}
		};
	};
</script>

{#if show}
	<div class="overlay" use:onShow on:click|self={close(onClose)} style="z-index: {zIndex};" transition:fade={{ duration: 150 }}>
		<div class="modal">
			{#if title}
				<h1>{title}</h1>
				<hr />
			{/if}
			<slot />
			{#if acceptButton || closeButton}
				<hr style="margin-top: 1em" />
			{/if}
			<div class="buttons row">
				{#if acceptButton}
					<Button icon={FaCheck} submit={close(onAccept)} success disabled={acceptDisable}>{acceptButton}</Button>
				{/if}
				{#if closeButton}
					<Button icon={FaTimes} submit={close(onClose)} error>{closeButton}</Button>
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	.overlay {
		position: fixed;
		top: 0;
		left: 0;
		width: 100%;
		height: 100%;
		display: grid;
		justify-content: center;
		align-items: center;
		overflow: auto;
		background: #0004;
	}
	.modal {
		background-color: var(--background);
		border: 0.15em solid var(--gray-1);
		padding: 1.5em;
		min-width: 30em;
		font-size: 1.1em;
	}
	h1 {
		margin-bottom: 0.25em;
	}
	hr + hr {
		display: none;
	}
</style>
