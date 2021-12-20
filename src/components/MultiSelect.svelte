<!-- original code https://github.com/janosh/svelte-multiselect/blob/main/src/lib/MultiSelect.svelte -->
<script>
	import { createEventDispatcher } from 'svelte';
	import IoMdCloseCircleOutline from 'svelte-icons/io/IoMdCloseCircleOutline.svelte';
	import FaArrowsAltV from 'svelte-icons/fa/FaArrowsAltV.svelte';
	export let selected;

	export let placeholder = ``;
	export let options;
	export let input = null;
	export let name = ``;
	export let noOptionsMsg = `No matching options`;

	export let removeBtnTitle = `Remove`;
	export let removeAllTitle = `Remove all`;

	if (!selected) selected = [];

	if (!(options?.length > 0)) console.error(`MultiSelect missing options`);

	const dispatch = createEventDispatcher();
	let activeOption, searchText;
	let showOptions = false;

	$: filteredOptions = searchText ? options.filter((option) => option.toLowerCase().includes(searchText.toLowerCase())) : options;
	$: if ((activeOption && !filteredOptions.includes(activeOption)) || (!activeOption && searchText)) activeOption = filteredOptions[0];

	function add(token) {
		if (!selected.includes(token)) {
			searchText = ``; // reset search string on selection
			selected = [token, ...selected];
		}
		if (selected.length == options.length) {
			setOptionsVisible(false);
		}
	}

	function remove(token) {
		if (typeof selected === `string`) return;
		selected = selected.filter((item) => item !== token);
	}

	function setOptionsVisible(show) {
		// nothing to do if visibility is already as intended
		if (show === showOptions) return;
		showOptions = show;
		activeOption = undefined;
		if (show) input?.focus();
	}

	function handleKeydown(event) {
		if (event.key === `Escape`) {
			setOptionsVisible(false);
			searchText = ``;
		} else if (event.key === `Enter`) {
			if (showOptions && activeOption) {
				selected.includes(activeOption) ? remove(activeOption) : add(activeOption);
				searchText = ``;
			} // no active option means the options are closed in which case enter means open
			else setOptionsVisible(true);
		} else if ([`ArrowDown`, `ArrowUp`].includes(event.key)) {
			const increment = event.key === `ArrowUp` ? -1 : 1;
			const newActiveIdx = filteredOptions.indexOf(activeOption) + increment;

			if (newActiveIdx < 0) {
				activeOption = filteredOptions[filteredOptions.length - 1];
			} else {
				if (newActiveIdx === filteredOptions.length) activeOption = filteredOptions[0];
				else activeOption = filteredOptions[newActiveIdx];
			}
		} else if (event.key === `Backspace`) {
			// only remove selected tags on backspace if if there are any and no searchText characters remain
			if (selected.length > 0 && searchText.length === 0) {
				selected = selected.slice(0, selected.length - 1);
			}
		}
	}

	const removeAll = () => {
		selected = [];
		searchText = ``;
	};

	$: isSelected = (option) => {
		if (!(selected?.length > 0)) return false;
		// nothing is selected if `selected` is the empty array or string
		else return selected.includes(option);
	};

	const handleEnterAndSpaceKeys = (handler) => (event) => {
		if ([`Enter`, `Space`].includes(event.code)) {
			event.preventDefault();
			handler();
		}
	};
</script>

<div class="multiselect" style={showOptions ? `z-index: 2;` : ``} on:mouseup|stopPropagation={() => setOptionsVisible(!showOptions)}>
	<span>
		<FaArrowsAltV />
	</span>
	<ul class="tokens">
		{#if selected?.length > 0}
			{#each selected as tag}
				<li on:mouseup|self|stopPropagation={() => setOptionsVisible(true)}>
					{tag}
					<button on:mouseup|stopPropagation={() => remove(tag)} on:keydown={handleEnterAndSpaceKeys(() => remove(tag))} type="button" title="{removeBtnTitle} {tag}">
						<IoMdCloseCircleOutline height="12pt" />
					</button>
				</li>
			{/each}
		{/if}
		{#if selected.length != options.length}
			<input
				autocomplete="off"
				bind:value={searchText}
				on:mouseup|self|stopPropagation={() => setOptionsVisible(true)}
				on:keydown={handleKeydown}
				on:focus={() => setOptionsVisible(true)}
				on:blur={() => dispatch(`blur`)}
				on:blur={() => setOptionsVisible(false)}
				{name}
				placeholder={selected.length ? `` : placeholder}
			/>
		{/if}
	</ul>
	<button
		type="button"
		class="remove-all"
		title={removeAllTitle}
		on:mouseup|stopPropagation={removeAll}
		on:keydown={handleEnterAndSpaceKeys(removeAll)}
		style={selected.length === 0 ? `display: none;` : ``}
	>
		<IoMdCloseCircleOutline height="14pt" />
	</button>

	{#key showOptions}
		<ul class="options" class:hidden={!showOptions}>
			{#each filteredOptions as option}
				<li
					on:mouseup|preventDefault|stopPropagation
					on:mousedown|preventDefault|stopPropagation={() => {
						isSelected(option) ? remove(option) : add(option);
					}}
					class:selected={isSelected(option)}
					class:active={activeOption === option}
				>
					{option}
				</li>
			{:else}
				{noOptionsMsg}
			{/each}
		</ul>
	{/key}
</div>

<style>
	.multiselect {
		position: relative;
		margin: 0 0.2em;
		transition: 200ms linear;
		background: #fff1;
		border: 2px solid #fff0;
		border-radius: 0.25em;
		min-height: 18pt;
		display: inline-flex;
		cursor: text;
		min-width: 15em;
	}
	.multiselect:hover {
		border-color: #fff8;
	}
	.multiselect:focus-visible {
		outline: 0.1em solid var(--green-2);
	}
	.multiselect:focus {
		background-color: var(--green-3);
		outline: none;
	}

	ul.tokens > li {
		background: var(--green-2);
		align-items: center;
		border-radius: 4pt;
		display: flex;
		margin: 2pt;
		padding: 0 0 0 1ex;
		height: 16pt;
	}
	ul.tokens > li button,
	button.remove-all,
	span {
		display: flex;
		cursor: pointer;
	}
	button {
		color: inherit;
		background: transparent;
		border: none;
		padding: 0 2pt;
	}
	ul.tokens > li button:hover,
	button.remove-all:hover,
	button:focus {
		color: var(--error);
	}

	.multiselect input {
		border: none;
		outline: none;
		width: 2em;
		background: none;
		flex: 1;
	}

	ul.tokens {
		display: flex;
		padding: 0;
		margin: 0;
		flex-wrap: wrap;
		flex: 1;
	}

	ul.options {
		list-style: none;
		max-height: 200px;
		padding: 0;
		margin: 2px;
		top: 100%;
		width: 100%;
		position: absolute;
		border-radius: 1ex;
		overflow: auto;
		background: #444;
	}
	ul.options.hidden {
		visibility: hidden;
	}
	ul.options li {
		padding: 3pt 2ex;
		cursor: pointer;
	}
	ul.options li.selected,
	ul.options li:not(.selected):hover {
		border-left: 4pt solid var(--green-2);
	}
	ul.options li.active {
		background: var(--green-2);
	}

	.multiselect ul.tokens > li button,
	.multiselect button.remove-all {
		/* buttons to remove a single or all selected options at once */
		width: 1.5em;
	}
	span {
		display: inline-flex;
		height: 0.75em;
		margin-left: 5px;
		margin-right: 5px;
		margin-top: auto;
		margin-bottom: auto;
		color: #fff;
	}
</style>
