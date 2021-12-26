<script>
	import Modal, { getModal } from './PopupModal.svelte';
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
			getModal('first').open(() => {
				changeValue = '';
			});
			openEdit = false;
		}
	}

	let changeValue = '';
	let changeOptions = '';
	const dispatch = createEventDispatcher();

	function close(id) {
		getModal(id).close();
		changeValue = '';
	}
	function confirm() {
		dispatch(editType, {
			value: changeValue,
			user: row,
			attribute: attributeToEdit
		});
		close('second');
		close('first');
	}
	$: isDisabled = (editType == 'add' || editType == 'delete') && changeOptions?.length === 0;
	$: {
		changeOptions = attributeToEdit?.[editType + 'Options'];
		if (typeof changeOptions == 'function') {
			changeOptions = changeOptions(row);
		}
	}
</script>

<Modal id="first">
	<h3>{typeText()} {attributeToEdit?.title?.toLocaleLowerCase()} for {row?.fullName}</h3>
	{#if editType == 'edit'}
		<p>Eksisterende verdi: {attributeToEdit?.value(row)}</p>
		{#if Array.isArray(changeOptions)}
			<select bind:value={changeValue}>
				<option></option>
				{#each changeOptions as v}
					<option value={v}>{v}</option>
				{/each}
			</select>
		{:else}
			<input bind:value={changeValue} />
		{/if}
	{:else if editType == 'add' || editType == 'delete'}
		{#if attributeToEdit?.value(row).length != 0}
			<span>Med i følgende {attributeToEdit?.title_plural}:</span>
			<ul style="margin-top:0">
				{#each attributeToEdit?.value(row) as v}
					<li>{v}</li>
				{/each}
			</ul>
		{:else}
			<p><i>Ikke medlem av noen {attributeToEdit?.title_plural}</i></p>
		{/if}
		{#if changeOptions?.length != 0}
			<div class="options">
				<p class="info">{attributeToEdit?.title}:</p>

				<select bind:value={changeValue}>
					<option></option>
					{#each changeOptions as option} 
						<option value={option.id}>{option.name}</option>
					{/each}
				</select>
			</div>
		{:else if editType == 'add'}
			<p><i>Brukeren er med i alle {attributeToEdit?.title_plural}</i></p>
		{/if}
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
