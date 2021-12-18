<script>
	import Modal, { getModal } from './PopupModal.svelte';
	import { createEventDispatcher } from 'svelte';
	export let type = '';
	export let row = {};
	export let attributeToEdit = undefined;

	let changeValue = '';
	const dispatch = createEventDispatcher();
	const getValueForUpdateQuery = () => {
		let user = '';
		if (type == 'Endre') {
			user = attributeToEdit.editConfirm(changeValue);
		} else if (type == 'Legg til') {
			user = null;
		}
		return { user };
	};
	function confirmedUpdate() {
		getModal('second').close();
		getModal('first').close();
		dispatch('confirm', {
			user: getValueForUpdateQuery(),
			row: row,
			type: type
		});
		changeValue = '';
	}
</script>

<Modal id="first">
	<h3>{type} {attributeToEdit?.title?.toLocaleLowerCase()} for {row?.fullName}</h3>
	{#if type == 'Endre'}
		<p>Eksisterende verdi: {attributeToEdit?.value(row)}</p>
		{#if Array.isArray(attributeToEdit?.edit)}
			<select bind:value={changeValue}>
				{#each attributeToEdit?.edit as v}
					<option value={v}>{v}</option>
				{/each}
			</select>
		{:else}
			<input bind:value={changeValue} style="display:block;margin:10px;" />
		{/if}
	{:else if type == 'Legg til'}
		{#if attributeToEdit?.value(row)}
			<span>Med i følgende {attributeToEdit?.title_plural}:</span>
			<ul style="margin-top:0">
				{#each attributeToEdit?.value(row) as v}
					<li>{v}</li>
				{/each}
			</ul>
		{:else}
			<p><i>Ikke medlem av noen {attributeToEdit?.title_plural}</i></p>
		{/if}
		<select>
			{#each attributeToEdit?.add(row) as v}
				<option value={v}>{v}</option>
			{/each}
			{#if attributeToEdit?.add(row).length == 0}
				<option value="" disabled selected>Allerede med i alle {attributeToEdit?.title_plural}</option>
			{/if}
		</select>
	{/if}
	<button on:click={() => getModal('second').open()}>
		{type}
	</button>
</Modal>
<Modal id="second">
	<h3>Er du sikker på at du vil endre denne verdien?</h3>
	<!-- Passing a value back to the callback function	 -->
	<div class="verification">
		<button on:click={() => confirmedUpdate()}> Bekreft </button>
		<button on:click={() => getModal('second').close()}> Avbryt </button>
	</div>
</Modal>

<style>
	.verification {
		display: block;
	}
	.verification > button {
		display: inline;
		width: fit-content;
	}
</style>
