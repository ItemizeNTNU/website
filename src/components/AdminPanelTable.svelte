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

	/** @type {Object} */
	export let filterSelections = {};

	// expand
	/** @type {Array.<string|number>} */
	export let expanded = [];

	/** @type {Object} */
	export let filterOptions = [];

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
	export let classNameRowExpanded = '';

	/** @type {string} class added to the expanded row*/
	export let classNameExpandedContent = '';

	/** @type {string} class added to the cell that allows expanding/closing */
	export let classNameCellExpand = '';

	const dispatch = createEventDispatcher();

	let sortFunction = () => '';
	let searchValue = '';
	// Validation
	if (!Array.isArray(expanded)) throw "'expanded' needs to be an array";

	let showFilterHeader = columns.some((c) => {
		// check if there are any filter or search headers
		return c.filterOptions !== undefined || c.searchValue !== undefined;
	});
	let filterValues = {};
	let filterOptionValues = {};
	Object.keys(filterOptions).forEach((key) => (filterOptionValues[key] = []));

	let columnByKey = {};
	$: {
		columnByKey = {};
		columns.forEach((col) => {
			columnByKey[col.key] = col;
		});
	}

	$: colspan = (showExpandIcon ? 1 : 0) + columns.length;

	$: c_rows = rows
		.filter((r) => {
			// get search and filter results/matches
			let resSearch = '' || columns.find((col) => col.searchValue && (col.searchValue(r) + '').toLocaleLowerCase().indexOf(searchValue.toLocaleLowerCase()) >= 0);
			let res =
				resSearch &&
				Object.keys(filterOptionValues).every((f) => {
					if (filterOptionValues[f].length == 0) return true;
					if (!r[f]) return false;
					return filterOptionValues[f].includes(r[f].toLocaleLowerCase());
				});
			return res;
		})
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

	const calculateFilterValues = () => {
		filterValues = {};
		columns.forEach((c) => {
			if (typeof c.filterOptions === 'function') {
				filterValues[c.key] = c.filterOptions(rows);
			} else if (Array.isArray(c.filterOptions)) {
				// if array of strings is provided, use it for name and value
				filterValues[c.key] = c.filterOptions.map((val) => ({
					name: val,
					value: val
				}));
			}
		});
	};

	$: {
		let col = columnByKey[sortBy];
		if (col !== undefined && col.sortable === true && typeof col.value === 'function') {
			sortFunction = (r) => col.value(r);
		}
	}

	$: {
		// if filters are enabled, watch rows and columns
		if (showFilterHeader && columns && rows) {
			calculateFilterValues();
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

	const handleClickRow = (event, row) => {
		dispatch('clickRow', { event, row });
	};

	const handleClickExpand = (event, row) => {
		row.$expanded = !row.$expanded;
		const keyVal = row[expandRowKey];
		if (expandSingle && row.$expanded) {
			expanded = [keyVal];
		} else if (expandSingle) {
			expanded = [];
		} else if (!row.$expanded) {
			expanded = expanded.filter((r) => r != keyVal);
		} else {
			expanded = [...expanded, keyVal];
		}
		dispatch('clickExpand', { event, row });
	};

	const handleClickCell = (event, row, key) => {
		dispatch('clickCell', { event, row, key });
	};
</script>

<div class="filterOptions" style="margin:10px;">
	{#if searchable}
		<div style="padding:2px;display:block">
			<p>Søk etter brukere:</p>
			<input bind:value={searchValue} />
		</div>
	{/if}
	{#each Object.keys(filterOptions) as key}
		<div style="display:block; padding: 2px; margin-left:10px;">
			<p>Filtrer på {key}</p>
			<MultiSelect --sms-options-bg="#666" bind:selected={filterOptionValues[key]} options={filterOptions[key]} placeholder={key} />
		</div>
	{/each}
	<p>Avansert søk</p>
</div>
<table class={asStringArray(classNameTable)}>
	<thead class={asStringArray(classNameThead)}>
		{#if showFilterHeader}
			<tr>
				{#each columns as col}
					<th>
						{#if filterValues[col.key] !== undefined}
							<select bind:value={filterSelections[col.key]} class={asStringArray(classNameSelect)}>
								<option value={undefined} />
								{#each filterValues[col.key] as option}
									<option value={option.value}>{option.name}</option>
								{/each}
							</select>
						{/if}
					</th>
				{/each}
				{#if showExpandIcon}
					<th />
				{/if}
			</tr>
		{/if}
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
				<tr
					on:click={(e) => {
						handleClickRow(e, row);
					}}
					class={asStringArray([classNameRow, row.$expanded && classNameRowExpanded])}
				>
					{#each columns as col}
						<td
							on:click={(e) => {
								handleClickCell(e, row, col.key);
							}}
							class={asStringArray([col.class, classNameCell])}
						>
							{#if col.renderComponent}
								<svelte:component this={col.renderComponent.component || col.renderComponent} {...col.renderComponent.props || {}} {row} {col} />
							{:else}
								{@html col.renderValue ? col.renderValue(row) : col.value(row)}
							{/if}
						</td>
					{/each}
					{#if showExpandIcon}
						<td on:click={(e) => handleClickExpand(e, row)} class={asStringArray(['isClickable', classNameCellExpand])}>
							{@html row.$expanded ? iconExpand : iconExpanded}
						</td>
					{/if}
				</tr>
				{#if row.$expanded}
					<tr class={asStringArray(classNameExpandedContent)}
						><td {colspan}>
							<slot name="expanded" {row} {n} />
						</td></tr
					>
				{/if}
			</slot>
		{/each}
	</tbody>
</table>

<style>
	.filterOptions {
		display: inline-flex;
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
	input {
		width: 250px;
		display: block;
		padding: 2px;
	}
	p {
		display: block;
		padding: 2px;
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
