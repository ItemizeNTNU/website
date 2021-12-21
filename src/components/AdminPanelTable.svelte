<!-- ORIGINAL SOURCE: https://github.com/dasDaniel/svelte-table/blob/develop/src/SvelteTable.svelte -->
<script>
	import MultiSelect from '../components/MultiSelect.svelte';
	import FaSortUp from 'svelte-icons/fa/FaSortUp.svelte';
	import FaSortDown from 'svelte-icons/fa/FaSortDown.svelte';
	import FaSort from 'svelte-icons/fa/FaSort.svelte';

	export let columns;
	export let rows;
	export let simpleFilterOptions = {};
	export let expandRowKey = null;
	export let filters = [];

	let sortBy = '';
	let sortOrder = 1;
	let expanded = [];

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

	let filterValues = {};
	filters.forEach((el) => (filterValues[el.title] = el.defaultFiltering));
	let columnByKey = {};
	$: {
		columnByKey = {};
		columns.forEach((col) => {
			columnByKey[col.key] = col;
		});
	}
	const unsetBoolFilter = (input, title) => {
		if(input.__value===filterValues[title]){
			filterValues[title] = ''
		}
	}

	$: colspan = 1 + columns.length;

	const simpleFilter = (rows, searchValue) => {
		page = 1;
		return rows.filter((r) => simpleFilterOptions.filter(r, searchValue));
	};
	const advancedFilter = (rows, filterValues) => {
		page = 1;
		return rows.filter((r) => filters.every((e) => e.filter(r, filterValues[e.title])));
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
		if (col !== undefined && typeof col.value === 'function') {
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
		{#each filters as filter}
			{#if filter.isDate}
				<span>
					<p>{filter.title} før:</p>
					<input onfocus="(this.type='date')" onblur="(this.type='text')" bind:value={filterValues[filter.title][0]} />
				</span>
				<span>
					<p>{filter.title} etter:</p>
					<input onfocus="(this.type='date')" onblur="(this.type='text')" bind:value={filterValues[filter.title][1]} />
				</span>
			{:else if filter.isBoolean}
				<span class="radio">
					<p>{filter.title}:</p>
					<label for="html">Ja</label>
					<input name="yes" value={true} on:click={unsetBoolFilter(this, filter.title)} bind:group={filterValues[filter.title]} type="radio">
					<label for="html">Nei</label>
					<input name="yes" value={undefined} on:click={unsetBoolFilter(this, filter.title)} bind:group={filterValues[filter.title]} type="radio">
				</span>
			{:else}
				<span>
					<p>{filter.title}:</p>
					{#if filter.filterValues}
						<MultiSelect bind:selected={filterValues[filter.title]} options={filter.filterValues} />
					{:else}
						<input bind:value={filterValues[filter.title]} />
					{/if}
				</span>
			{/if}
		{/each}
	{/if}
	<br />

	<button on:click={() => (advancedSearch = !advancedSearch)}>{advancedSearch ? 'Skjul a' : 'A'}vansert søk</button>
</div>
<table>
	<thead>
		<tr>
			{#each columns as col}
				<th on:click={(e) => handleClickCol(e, col)} class="pointer header">
					{col.title}
					<span>
					{#if sortBy === col.key}
						{#if sortOrder === 1}
							<FaSortUp />
						{:else}
							<FaSortDown />
						{/if}
					{:else}
						<FaSort />
					{/if}
					</span>
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
	.radio > input{
		width: min-content;
		transition: none;
		background-color: #777;
		margin-right: 1em;
	}
	.radio > input:checked{
		background-color: var(--green-2);
	}
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
		width: 100%;
		position: relative;
		display: grid;
		grid-template-columns: 50% 50%;
		padding: 0.6em;
		padding-bottom: 2em;
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

	.header > span {
		display: inline-block;
		height: 0.75em;
		vertical-align: sub;
		margin-left: 4px;
		color: #fff;
		transition: 0.4s linear;
	}
	.header > span:hover {
		color: var(--green-2);
	}
</style>
