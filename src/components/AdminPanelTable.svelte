<!-- ORIGINAL SOURCE: https://github.com/dasDaniel/svelte-table/blob/develop/src/SvelteTable.svelte -->
<script>
	import MultiSelect from '../components/MultiSelect.svelte';

	/** @type {Array<Object>} */
	export let columns;

	/** @type {Array<Object>} */
	export let rows;

	// READ AND WRITE
	/** @type {Array<Object>} */
	export let advancedFilterOptions = [];

	/** @type {Object} */
	export let simpleFilterOptions = {};

	// READ ONLY

	/** @type {string} */
	export let expandRowKey = null;

	let sortBy = '';
	let sortOrder = 1;
	let expanded = [];

	let iconAsc = '▲';
	let iconDesc = '▼';

	let iconExpand = '⌄';
	let iconExpanded = '⌃';
	let maxPerPage = 10;
	let page = 1;
	let advancedSearch = false;
	let sortFunction = () => '';
	let searchValue = '';

	const increasePage = () => {
		page += page * maxPerPage >= c_rows.length ? 0 : 1;
	};
	const decreasePage = () => {
		page -= page == 1 ? 0 : 1;
	};

	// Validation
	if (!Array.isArray(expanded)) throw "'expanded' needs to be an array";

	let filterValues = {};
	advancedFilterOptions.forEach((el) => (filterValues[el.name] = el.default));

	let columnByKey = {};
	$: {
		columnByKey = {};
		columns.forEach((col) => {
			columnByKey[col.key] = col;
		});
	}

	$: colspan = 1 + columns.length;

	const simpleFilter = (rows, searchValue) => {
		page = 1;
		return rows.filter((r) => simpleFilterOptions.filter(r, searchValue));
	};
	const advancedFilter = (rows, filters) => {
		page = 1;
		return rows.filter((r) => advancedFilterOptions.every((e) => e.filter(r, filters[e.name])));
	};
	// Filter rows
	$: c_rows = (advancedSearch ? advancedFilter(rows, filterValues) : simpleFilter(rows, searchValue))
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

	const handleClickCol = (event, col) => {
		sortOrder = col.key === sortBy ? sortOrder * -1 : 1;
		sortBy = col.key;
	};

	const handleClickExpand = (event, row) => {
		row.$expanded = !row.$expanded;
		if (row.$expanded) {
			expanded = [row[expandRowKey]];
		} else {
			expanded = [];
		}
	};
</script>

<div class="filterOptions">
	{#if !advancedSearch}
		<p>{simpleFilterOptions.title}</p>
		<input bind:value={searchValue} />
	{:else}
		{#each advancedFilterOptions as filter}
			<span>
				<p>{filter.name}</p>
				{#if filter.values}
					<MultiSelect --sms-options-bg="#666" bind:selected={filterValues[filter.name]} options={filter.values} />
				{:else if filter.isDate}
					<input onfocus="(this.type='date')" onblur="(this.type='text')" bind:value={filterValues[filter.name]} />
				{:else}
					<input bind:value={filterValues[filter.name]} />
				{/if}
			</span>
		{/each}
	{/if}
	<br />

	<button on:click={() => (advancedSearch = !advancedSearch)}>{advancedSearch ? 'Skjul a' : 'A'}vansert søk</button>
</div>
<table>
	<thead>
		<tr>
			{#each columns as col}
				<th on:click={(e) => handleClickCol(e, col)} class="pointer">
					{col.title}
					{#if sortBy === col.key}
						{@html sortOrder === 1 ? iconAsc : iconDesc}
					{/if}
				</th>
			{/each}
			<th />
		</tr>
	</thead>

	<tbody>
		{#each c_rows.slice((page - 1) * maxPerPage, page * maxPerPage) as row, n}
			<tr on:click={(e) => handleClickExpand(e, row)} class="expand pointer">
				{#each columns as col}
					<td>{col.value(row)}</td>
				{/each}
				<td>{@html row.$expanded ? iconExpand : iconExpanded}</td>
			</tr>
			{#if row.$expanded}
				<tr>
					<td {colspan}><slot name="expanded" {row} {n} /></td>
				</tr>
			{/if}
		{/each}
	</tbody>
</table>
<div class="pageination" style="display:grid;grid-template-columns: 33% 33% 34% ">
	<p style="text-align:left" on:click={decreasePage}>←</p>
	<p style="text-align:center">{(page - 1) * maxPerPage + (c_rows.length != 0 ? 1 : 0)}-{Math.min(page * maxPerPage, c_rows.length)} av {c_rows.length}</p>

	<p style="text-align:right " on:click={increasePage}>→</p>
	<span>
		<p>Resultater per side:</p>
		<select bind:value={maxPerPage} style="margin-left:10px;width:70px">
			{#each [5, 10, 15, 20, 40, 60, 100] as num}
				<option value={num}>{num}</option>
			{/each}
		</select>
	</span>
</div>

<style>
	.pageination p,
	select {
		margin: 0px;
		margin-top: 15px;
	}
	.pageination span * {
		display: inline;
	}
	.filterOptions p {
		display: inline;
		margin: 0px;
		margin-right: 10px;
	}
	.filterOptions span {
		display: inline;
		width: 100%;
		padding: 0.4em;
	}
	button {
		position: absolute;
		right: 0;
		bottom: 0;
		font-size: smaller;
		width: auto;
	}
	.filterOptions {
		padding: 5px;
		width: 100%;
		position: relative;
		display: grid;
		grid-template-columns: 50% 50%;
		align-items: top;
		padding: 0.6em;
	}
	table {
		width: 100%;
	}
	.pointer {
		cursor: pointer;
	}
	tbody,
	td {
		border: 0px solid #666;
		border-bottom-width: 1px;
	}
	th {
		border: 0px solid #bbb;
		border-bottom-width: 2px;
		text-align: left;
	}
	table {
		caption-side: bottom;
		border-collapse: collapse;
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
