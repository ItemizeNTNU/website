<script context="module">
</script>

<script>
	import Modal, { getModal } from './PopupModal.svelte';
	import MultiSelect from '../components/MultiSelect.svelte';
	import { createEventDispatcher } from 'svelte';
	export let editType = '';
	export let row = {};
	export let attributeToEdit = undefined;
	export let openEdit;

	$: typeText = () => {
		if (editType == 'edit') return 'Endre';
		else if (editType == 'add') return 'Legg til';
		else if (editType == 'delete') return 'Fjern';
	};

	$: {
		if (openEdit) {
			getModal('first').open(callback);
			openEdit = false;
		}
	}
	const callback = () => {
		changeValue = '';
		selectedMulti = [];
	};

	let changeValue = '';
	let selectedMulti = [];
	let changeOptions = '';
	const dispatch = createEventDispatcher();

	function close(id) {
		getModal(id).close();
		changeValue = '';
		selectedMulti = [];
	}
	function confirm() {
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
			let del;
			if (attributeToEdit?.key == 'groupIds') {
				del = attributeToEdit.deleteConfirm(row.id, changeValue);
			} else if (attributeToEdit?.key == 'applicationRoles') {
				del = attributeToEdit.deleteConfirm(row.applicationRoles, changeValue, selectedMulti);
			}
			dispatch('delete', {
				delete: del,
				row: row,
				attribute: attributeToEdit?.key
			});
		}
		close('second');
		close('first');
	}
	$: isDisabled = (editType == 'add' && changeOptions?.length == 0) || (editType == 'delete' && changeOptions?.length == 0);
	$: {
		changeOptions = attributeToEdit?.[editType + 'Options'];
		if (typeof changeOptions == 'function') {
			changeOptions = changeOptions(row);
		}
		console.log(changeOptions);
	}
</script>

<Modal id="first">
	<h3>{typeText()} {attributeToEdit?.title?.toLocaleLowerCase()} for {row?.fullName}</h3>
	{#if editType == 'edit'}
		<p>Eksisterende verdi: {attributeToEdit?.value(row)}</p>
		{#if Array.isArray(changeOptions)}
			<select bind:value={changeValue}>
				{#each changeOptions as v}
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
				{#each changeOptions as option}
					<option value={option.id}>{option.name}</option>
				{/each}
				{#if changeOptions?.length == 0}
					<option value="" disabled>Kan ikke {typeText()?.toLocaleLowerCase()}{editType == 'delete' ? 'e' : ''} {attributeToEdit?.title_plural}</option>
				{/if}
			</select>
			{#if attributeToEdit?.key == 'applicationRoles'}
				<br />
				<p class="info">Roller:</p>
				<MultiSelect
					bind:selected={selectedMulti}
					options={changeOptions?.find((a) => a.id == changeValue)?.roles || []}
					noOptionsMsg="Ingen roller tilgjenlig for bruker for applikasjonen"
				/>
				{#if editType == 'delete'}
					<br />
					<p style="font-size:smaller"><i>Ved å ikke velge noen roller vil brukeren miste tilgang til hele applikasjonen</i></p>
				{/if}
			{/if}
		</div>
	{/if}
	<button class="do" disabled={isDisabled} on:click={() => getModal('second').open()}>
		{typeText()}
	</button>
</Modal>
<Modal id="second">
	<h3>Er du sikker på at du vil {typeText()?.toLocaleLowerCase()} denne verdien?</h3>
	<div class="verification">
		<button on:click={() => confirm()}> Bekreft </button>
		<button on:click={() => close('second')}> Avbryt </button>
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
