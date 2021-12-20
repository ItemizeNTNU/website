<script>
	import Modal, { getModal } from './PopupModal.svelte';
	import MultiSelect from '../components/MultiSelect.svelte';
	import { createEventDispatcher } from 'svelte';
	export let editType = '';
	export let row = {};
	export let attributeToEdit = undefined;

	$: typeText = () => {
		if (editType == 'edit') return 'Endre';
		else if (editType == 'add') return 'Legg til';
		else if (editType == 'delete') return 'Fjern';
	};

	let changeValue = '';
	let selectedMulti = [];
	const dispatch = createEventDispatcher();

	function confirm() {
		getModal('second').close();
		getModal('first').close();
		if (editType == 'edit') {
			dispatch('edit', {
				edit: attributeToEdit.editConfirm(changeValue),
				row: row
			});
		} else if (editType == 'add') {
			let add;
			if (attributeToEdit?.key == 'groupIds') {
				add = attributeToEdit.addConfirm(row.id, changeValue);
			} else if (attributeToEdit?.key == 'applicationRoles') {
				add = attributeToEdit.addConfirm(changeValue, selectedMulti);
			}
			dispatch('add', {
				add: add,
				row: row,
				attribute: attributeToEdit?.key
			});
		} else if (editType == 'delete') {
			dispatch('delete', {
				delete: attributeToEdit.deleteConfirm(row.id, changeValue),
				row: row
			});
		}
		changeValue = '';
		selectedMulti = [];
	}
	$: isDisabled = (editType == 'add' && attributeToEdit?.add(row)?.length == 0) || (editType == 'delete' && attributeToEdit?.delete(row)?.length == 0);
</script>

<Modal id="first">
	<h3>{typeText()} {attributeToEdit?.title?.toLocaleLowerCase()} for {row?.fullName}</h3>
	{#if editType == 'edit'}
		<p>Eksisterende verdi: {attributeToEdit?.value(row)}</p>
		{#if Array.isArray(attributeToEdit?.edit)}
			<select bind:value={changeValue}>
				{#each attributeToEdit?.edit as v}
					<option value={v}>{v}</option>
				{/each}
			</select>
		{:else}
			<input bind:value={changeValue} />
		{/if}
	{:else if editType == 'add' || editType == 'delete'}
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
		<div class="options">
			<p class="info">{attributeToEdit?.title}:</p>
			<select bind:value={changeValue}>
				{#if editType == 'add'}
					{#each attributeToEdit?.add(row) as option}
						<option value={option.id}>{option.name}</option>
					{/each}
					{#if attributeToEdit?.add(row).length == 0}
						<option value="" disabled>Allerede med i alle {attributeToEdit?.title_plural}</option>
					{/if}
				{:else}
					{#each attributeToEdit?.delete(row) as option}
						<option value={option.id}>{option.name}</option>
					{/each}
					{#if attributeToEdit?.delete(row).length == 0}
						<option value="" disabled>Ikke med i noen {attributeToEdit?.title_plural}</option>
					{/if}
				{/if}
			</select>
			{#if attributeToEdit?.key == 'applicationRoles'}
				<br />
				<p class="info">Roller:</p>
				<MultiSelect
					bind:selected={selectedMulti}
					options={attributeToEdit?.add(row)?.find((a) => a.id == changeValue)?.roles || []}
					noOptionsMsg="Ingen roller tilgjenlig for bruker for applikasjonen"
				/>
			{/if}
		</div>
	{/if}
	<button class="do" disabled={isDisabled} on:click={() => getModal('second').open()}>
		{typeText()}
	</button>
</Modal>
<Modal id="second">
	<h3>Er du sikker på at du vil endre denne verdien?</h3>
	<!-- Passing a value back to the callback function	 -->
	<div class="verification">
		<button on:click={() => confirm()}> Bekreft </button>
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
	button:disabled,
	button:disabled:hover {
		border: inherit;
		background-color: #999;
		color: #666666;
	}
	.info {
		display: inline;
	}
	.options {
		margin: 5px;
		padding: 5px;
	}
	.do {
		width: fit-content;
		float: right;
	}
</style>
