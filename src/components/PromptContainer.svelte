<script context="module">
	export const prompt = (inputs) => {};
</script>

<script>
	import { prompts } from '../utils/prompt';
	import Modal from '../components/Modal.svelte';

	const submit = (prompt) => {
		if (!prompt.acceptDisable) {
			prompt.show = false;
			prompt.accept();
			$prompts = $prompts;
		}
	};
	const validate = (prompt) => {
		if (prompt.validate) {
			return () => {
				prompt.acceptDisable = !prompt.validate(prompt.inputs);
				$prompts = $prompts;
			};
		}
	};
</script>

{#each $prompts as prompt}
	<Modal
		bind:show={prompt.show}
		title={prompt.title}
		acceptButton={prompt.acceptText}
		acceptDisable={prompt.acceptDisable}
		closeButton="Close"
		onAccept={prompt.accept}
		onClose={prompt.close}>
		{#if prompt.body}
			<p>{@html prompt.body}</p>
		{/if}
		{#if prompt.inputs?.length}
			<form class="inputs" on:submit|preventDefault={submit(prompt)}>
				{#each prompt.inputs as input}
					<p class="name">{input.name}</p>
					<!-- svelte-ignore a11y-autofocus -->
					<input bind:value={input.value} on:input={validate(prompt)} autofocus={prompt.inputs.length == 1} />
				{/each}
			</form>
		{/if}
		{#if prompt.pre}
			<code class="block">{prompt.pre}</code>
		{/if}
	</Modal>
{/each}

<style>
	.inputs {
		display: grid;
		align-items: center;
		grid-template-columns: auto 1fr;
	}
	.name {
		padding-right: 0.5em;
	}
</style>
