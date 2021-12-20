<!-- original code https://github.com/janosh/svelte-multiselect/blob/main/src/lib/MultiSelect.svelte -->
<script>
	import { createEventDispatcher } from 'svelte';

	import { CrossIcon, ExpandIcon } from '../icons/Index';

	export let selected;

	export let placeholder = ``;
	export let options;
	export let input = null;
	export let name = ``;
	export let noOptionsMsg = `No matching options`;

	export let outerDivClass = ``;
	export let ulTokensClass = ``;
	export let liTokenClass = ``;
	export let ulOptionsClass = ``;
	export let liOptionClass = ``;

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

<!-- z-index: 2 when showOptions is true ensures the ul.tokens of one <MultiSelect /> display above those of another following shortly after it -->
<div class="multiselect {outerDivClass}" style={showOptions ? `z-index: 2;` : ``} on:mouseup|stopPropagation={() => setOptionsVisible(!showOptions)}>
	<ExpandIcon height="14pt" style="padding-left: 1pt;" />
	<ul class="tokens {ulTokensClass}">
		{#if selected?.length > 0}
			{#each selected as tag}
				<li class={liTokenClass} on:mouseup|self|stopPropagation={() => setOptionsVisible(true)}>
					{tag}
					<button on:mouseup|stopPropagation={() => remove(tag)} on:keydown={handleEnterAndSpaceKeys(() => remove(tag))} type="button" title="{removeBtnTitle} {tag}">
						<CrossIcon height="12pt" />
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
		<CrossIcon height="14pt" />
	</button>

	{#key showOptions}
		<ul class="options {ulOptionsClass}" class:hidden={!showOptions}>
			{#each filteredOptions as option}
				<li
					on:mouseup|preventDefault|stopPropagation
					on:mousedown|preventDefault|stopPropagation={() => {
						isSelected(option) ? remove(option) : add(option);
					}}
					class:selected={isSelected(option)}
					class:active={activeOption === option}
					class={liOptionClass}
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
	input {
		box-sizing: content-box;
	}
	input:focus-visible {
		outline: 0.1em solid transparent;
		background-color: transparent;
	}
	:where(.multiselect) {
		position: relative;
		margin: 1em 0;
		border: var(--sms-border, 1pt solid lightgray);
		border-radius: var(--sms-border-radius, 5pt);
		align-items: center;
		min-height: 18pt;
		display: inline-flex;
		cursor: text;
		margin: 5px;
		min-width: 250px;
	}
	ul {
		margin: 0px;
	}
	:where(.multiselect:focus-within) {
		border: var(--sms-focus-border, 1pt solid var(--sms-active-color, cornflowerblue));
	}

	:where(ul.tokens > li) {
		background: var(--sms-token-bg, var(--sms-active-color, cornflowerblue));
		align-items: center;
		border-radius: 4pt;
		display: flex;
		margin: 2pt;
		padding: 0 0 0 1ex;
		white-space: nowrap;
		height: 16pt;
	}
	:where(ul.tokens > li button, button.remove-all) {
		align-items: center;
		border-radius: 50%;
		display: flex;
		cursor: pointer;
	}
	:where(button) {
		color: inherit;
		background: transparent;
		border: none;
		cursor: pointer;
		outline: none;
		padding: 0 2pt;
	}
	:where(ul.tokens > li button:hover, button.remove-all:hover) {
		color: var(--sms-remove-x-hover-focus-color, lightskyblue);
	}
	:where(button:focus) {
		color: var(--sms-remove-x-hover-focus-color, lightskyblue);
		transform: scale(1.04);
	}

	:where(.multiselect input) {
		border: none;
		outline: none;
		width: 2em;
		display: inline-flex;
		background: none;
		color: var(--sms-text-color, inherit);
		flex: 1;
		min-width: 2em;
	}

	:where(ul.tokens) {
		display: flex;
		padding: 0;
		margin: 0;
		flex-wrap: wrap;
		flex: 1;
	}

	:where(ul.options) {
		list-style: none;
		max-height: 200px;
		padding: 0;
		top: 100%;
		width: 100%;
		position: absolute;
		border-radius: 1ex;
		overflow: auto;
		background: var(--sms-options-bg, #666);
	}
	:where(ul.options.hidden) {
		visibility: hidden;
	}
	:where(ul.options li) {
		padding: 3pt 2ex;
		cursor: pointer;
	}
	:where(ul.options li.selected) {
		border-left: var(--sms-li-selected-border-left, 3pt solid var(--sms-selected-color, green));
		background: var(--sms-li-selected-bg, inherit);
		color: var(--sms-li-selected-color, inherit);
	}
	:where(ul.options li:not(.selected):hover) {
		border-left: var(--sms-li-not-selected-hover-border-left, 3pt solid var(--sms-active-color, cornflowerblue));
		border-left: 3pt solid var(--blue);
	}
	:where(ul.options li.active) {
		background: var(--sms-li-active-bg, var(--sms-active-color, cornflowerblue));
	}
	:where(ul.options li.disabled) {
		background: var(--sms-li-disabled-bg, #f5f5f6);
		color: var(--sms-li-disabled-text, #b8b8b8);
		cursor: not-allowed;
	}
	:where(ul.options li.disabled:hover) {
		border-left: unset;
	}

	.multiselect ul.tokens > li button,
	.multiselect button.remove-all {
		/* buttons to remove a single or all selected options at once */
		width: 1.5em;
		display: inline-flex;
	}
</style>
