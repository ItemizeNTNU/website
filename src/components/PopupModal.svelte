<script context="module">
	let onTop; //keeping track of which open modal is on top
	const modals = {}; //all modals get registered here for easy future access

	// 	returns an object for the modal specified by `id`, which contains the API functions (`open` and `close` )
	export function getModal(id = '') {
		return modals[id];
	}
</script>

<script>
	import { onDestroy } from 'svelte';
	import IoMdCloseCircleOutline from 'svelte-icons/io/IoMdCloseCircleOutline.svelte';
	let topDiv;
	let visible = false;
	let prevOnTop;
	let closeCallback;

	export let id = '';

	function keyPress(ev) {
		//only respond if the current modal is the top one
		if (ev.key == 'Escape' && onTop == topDiv) close(); //ESC
	}

	/**  API **/
	function open(callback) {
		closeCallback = callback;
		if (visible) return;
		prevOnTop = onTop;
		onTop = topDiv;
		window.addEventListener('keydown', keyPress);

		//this prevents scrolling of the main window on larger screens
		document.body.style.overflow = 'hidden';

		visible = true;
		//Move the modal in the DOM to be the last child of <BODY> so that it can be on top of everything
		document.body.appendChild(topDiv);
	}

	function close(retVal) {
		if (!visible) return;
		window.removeEventListener('keydown', keyPress);
		onTop = prevOnTop;
		if (onTop == null) document.body.style.overflow = '';
		visible = false;
		if (closeCallback) closeCallback(retVal);
	}

	modals[id] = { open, close };

	onDestroy(() => {
		window.removeEventListener('keydown', keyPress);
	});
</script>

<div id="topModal" class:visible bind:this={topDiv} on:click={() => close()}>
	<div id="modal" on:click|stopPropagation={() => {}}>
		<span class="red" on:click={() => close()}>
			<IoMdCloseCircleOutline />
		</span>
		<div id="modal-content">
			<slot />
		</div>
	</div>
</div>

<style>
	#topModal {
		visibility: hidden;
		z-index: 9999;
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: #4448;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	#modal {
		position: relative;
		border-radius: 6px;
		background: var(--background);
		border: 2px solid #000;
		filter: drop-shadow(3px 3px 3px #555);
		padding: 1.5em;
	}

	.visible {
		visibility: visible !important;
	}
	#modal > span {
		position: absolute;
		top: -12px;
		right: -12px;
		width: 24px;
		height: 24px;
		cursor: pointer;
		transition: 0.1s linear;
	}
	span.red:hover {
		color: var(--error);
	}
	#modal-content {
		max-width: 30vw;
		max-height: calc(100vh);
	}
</style>
