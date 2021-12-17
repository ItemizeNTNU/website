<!-- ORIGINAL SOURCE: https://github.com/dasDaniel/svelte-table/blob/develop/src/SvelteTable.svelte -->
<script>
	import { createEventDispatcher } from 'svelte';
	import MultiSelect from '../components/MultiSelect.svelte';

	/** @type {Array<Object>} */
	export let columns;

	/** @type {Array<Object>} */
	export let rows;

	// READ AND WRITE

	/** @type {string} */
	export let sortBy = '';

	/** @type {number} */
	export let sortOrder = 1;

	// expand
	/** @type {Array.<string|number>} */
	export let expanded = [];

	/** @type {Array<Object>} */
	export let advancedFilterOptions = [];

	/** @type {Object} */
	export let simpleFilterOptions = {};

	// READ ONLY

	/** @type {string} */
	export let expandRowKey = null;

	/** @type {string} */
	export let expandSingle = false;

	/** @type {string} */
	export let searchable = true;

	/** @type {string} */
	export let iconAsc = '▲';

	/** @type {string} */
	export let iconDesc = '▼';

	/** @type {string} */
	export let iconExpand = '▼';

	/** @type {string} */
	export let iconExpanded = '▲';

	/** @type {boolean} */
	export let showExpandIcon = false;

	/** @type {string} */
	export let classNameTable = '';

	/** @type {string} */
	export let classNameThead = '';

	/** @type {string} */
	export let classNameTbody = '';

	/** @type {string} */
	export let classNameSelect = '';

	/** @type {string} */
	export let classNameRow = '';

	/** @type {string} */
	export let classNameCell = '';

	/** @type {string} class added to the expanded row*/
	export let classNameRowExpanded = 'expanded';

	/** @type {string} class added to the expanded row*/
	export let classNameExpandedContent = '';

	/** @type {string} class added to the cell that allows expanding/closing */
	export let classNameCellExpand = '';

	const dispatch = createEventDispatcher();
	let advancedSearch = false;
	let sortFunction = () => '';
	let searchValue = '';
	// Validation
	if (!Array.isArray(expanded)) throw "'expanded' needs to be an array";

	let filterOptionValues = {};
	advancedFilterOptions.forEach((el) => (filterOptionValues[el.name] = el.default));

	let columnByKey = {};
	$: {
		columnByKey = {};
		columns.forEach((col) => {
			columnByKey[col.key] = col;
		});
	}

	$: colspan = (showExpandIcon ? 1 : 0) + columns.length;

	const simpleFilter = (rows, searchValue) => {
		return rows.filter((r) => simpleFilterOptions.filter(r, searchValue));
	};
	const advancedFilter = (rows, filters) => {
		return rows.filter((r) => advancedFilterOptions.every((e) => e.filter(r, filters[e.name])));
	};
	// Find rows that match search
	$: c_rows = (advancedSearch ? advancedFilter(rows, filterOptionValues) : simpleFilter(rows, searchValue))
		.map((r) =>
			Object.assign({}, r, {
				// internal row property for sort order
				$sortOn: sortFunction(r),
				// internal row property for expanded rows
				$expanded: expandRowKey !== null && expanded.indexOf(r[expandRowKey]) >= 0
			})
		)
		.sort((a, b) => {
			if (a.$sortOn > b.$sortOn) return sortOrder;
			else if (a.$sortOn < b.$sortOn) return -sortOrder;
			return 0;
		});

	const asStringArray = (v) =>
		[]
			.concat(v)
			.filter((v) => typeof v === 'string' && v !== '')
			.join(' ');

	$: {
		let col = columnByKey[sortBy];
		if (col !== undefined && col.sortable === true && typeof col.value === 'function') {
			if (typeof col.value(rows[0]) === 'string') {
				sortFunction = (r) => col.value(r).toLocaleLowerCase();
			} else {
				sortFunction = (r) => col.value(r);
			}
		}
	}

	const updateSortOrder = (colKey) => {
		if (colKey === sortBy) {
			sortOrder = sortOrder === 1 ? -1 : 1;
		} else {
			sortOrder = 1;
		}
	};

	const handleClickCol = (event, col) => {
		if (col.sortable) {
			updateSortOrder(col.key);
			sortBy = col.key;
		}
		dispatch('clickCol', { event, col, key: col.key });
	};

	const handleClickExpand = (event, row) => {
		row.$expanded = !row.$expanded;
		const keyVal = row[expandRowKey];
		if (row.$expanded) {
			expanded = [keyVal];
		} else {
			expanded = [];
		}
	};
</script>

<div class="filterOptions" style="padding:5px;width:100%; position:relative">
	{#if !advancedSearch}
		<p style="display: inline;margin:0;margin-right: 10px;">{simpleFilterOptions.title}</p>
		<input style="display: inline" bind:value={searchValue} />
	{:else}
		<div class="advanced-search">
			{#each advancedFilterOptions as filter}
				<span>
					<p>{filter.name}</p>
					{#if filter.values}
						<MultiSelect --sms-options-bg="#666" bind:selected={filterOptionValues[filter.name]} options={filter.values} />
					{:else if filter.isDate}
						<input onfocus="(this.type='date')" onblur="(this.type='text')" bind:value={filterOptionValues[filter.name]} />
					{:else}
						<input bind:value={filterOptionValues[filter.name]} />
					{/if}
				</span>
			{/each}
		</div>
	{/if}
	<br />

	<button on:click={() => (advancedSearch = !advancedSearch)} style="position:absolute; right:0; bottom:0;font-size:smaller;width:auto"
		>{advancedSearch ? 'Skjul a' : 'A'}vansert søk</button
	>
</div>
<table class={asStringArray(classNameTable)}>
	<thead class={asStringArray(classNameThead)}>
		<slot name="header" {sortOrder} {sortBy}>
			<tr>
				{#each columns as col}
					<th on:click={(e) => handleClickCol(e, col)} class={asStringArray([col.sortable ? 'isSortable' : '', col.headerClass])}>
						{col.title}
						{#if sortBy === col.key}
							{@html sortOrder === 1 ? iconAsc : iconDesc}
						{/if}
					</th>
				{/each}
				{#if showExpandIcon}
					<th />
				{/if}
			</tr>
		</slot>
	</thead>

	<tbody class={asStringArray(classNameTbody)}>
		{#each c_rows as row, n}
			<slot name="row" {row} {n}>
				<tr on:click={(e) => handleClickExpand(e, row)} class={asStringArray([classNameRow, row.$expanded && 'expanded'])}>
					{#each columns as col}
						<td class={asStringArray([col.class, classNameCell])}>
							{col.value(row)}
						</td>
					{/each}
					{#if showExpandIcon}
						<td class={asStringArray(['isClickable', classNameCellExpand])}>
							{@html row.$expanded ? iconExpand : iconExpanded}
						</td>
					{/if}
				</tr>
				{#if row.$expanded}
					<tr class={asStringArray(classNameExpandedContent)}>
						<td {colspan}>
							<slot name="expanded" {row} {n} />
						</td>
					</tr>
				{/if}
			</slot>
		{/each}
	</tbody>
</table>

<style>
	.filterOptions p,
	input {
		display: inline;
	}
	table {
		width: 100%;
	}
	.isSortable {
		cursor: pointer;
	}

	.isClickable {
		cursor: pointer;
	}
	tbody,
	td,
	th {
		border-color: #666;
		border-style: solid;
		border-width: 0;
		border-bottom-width: 1px;
	}
	th {
		border-color: #bbb;
		border-bottom-width: 2px;
		text-align: left;
	}
	table {
		caption-side: bottom;
		border-collapse: collapse;
	}

	.advanced-search {
		display: grid;
		grid-template-columns: 50% 50%;
		align-items: top;
		padding: 0.6em;
	}
	.advanced-search span {
		padding: 0.4em;
	}

	:global(.multiselect ul.tokens > li button),
	:global(.multiselect button.remove-all) {
		/* buttons to remove a single or all selected options at once */
		width: 1.5em;
		display: inline-flex;
	}
	:global(.multiselect ul input) {
		width: 2em;
		display: inline-flex;
	}
	:global(.multiselect) {
		display: inline-flex;
		margin: 0px;
		min-width: 250px;
	}
</style>
